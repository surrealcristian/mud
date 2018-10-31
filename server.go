package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"text/scanner"
)

// User

type User struct {
	name     string
	password string
}

func NewUser(name, password string) *User {
	return &User{name: name, password: password}
}

func (u *User) Name() string {
	return u.name
}

func (u *User) Password() string {
	return u.password
}

// UserManager

type UserManager struct {
	users     map[*User]bool
	nameIndex map[string]*User
	mux       sync.RWMutex
}

func NewUserManager() *UserManager {
	return &UserManager{
		users:     make(map[*User]bool),
		nameIndex: make(map[string]*User),
	}
}

func (m *UserManager) Add(user *User) {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.users[user] = true
	m.nameIndex[user.name] = user
}

func (m *UserManager) Remove(user *User) {
	m.mux.Lock()
	defer m.mux.Unlock()

	delete(m.users, user)
}

func (m *UserManager) GetByName(name string) *User {
	if user, ok := m.nameIndex[name]; ok {
		return user
	}

	return nil
}

// ConnectCmd

const (
	ConnectCmdStr = "connect"
)

var (
	ErrCmdBadFormat = errors.New("cmd: invalid command")
)

type ConnectCmd struct {
	username string
	password string
}

func NewConnectCmd(username, password string) *ConnectCmd {
	return &ConnectCmd{
		username: username,
		password: password,
	}
}

func (c *ConnectCmd) Username() string {
	return c.username
}

func (c *ConnectCmd) Password() string {
	return c.password
}

func ConnectCmdUsage() string {
	return "usage: connect <username> <password>"
}

// DisconnectCmd

const (
	DisconnectCmdStr = "disconnect"
)

type DisconnectCmd struct{}

func NewDisconnectCmd() *DisconnectCmd {
	return &DisconnectCmd{}
}

func DisconnectCmdUsage() string {
	return "usage: disconnect"
}

// SayCmd

const (
	SayCmdStr = "say"
)

type SayCmd struct {
	text string
}

func NewSayCmd(text string) *SayCmd {
	return &SayCmd{
		text: text,
	}
}

func (c *SayCmd) Text() string {
	return c.text
}

// UnknownCmd

type UnknownCmd struct {
	text string
}

func NewUnknownCmd(text string) *UnknownCmd {
	return &UnknownCmd{
		text: text,
	}
}

func (c *UnknownCmd) Text() string {
	return c.text
}

func ParseConnectCmd(msgScanner *scanner.Scanner) (*ConnectCmd, error) {
	if tok := msgScanner.Scan(); tok == scanner.EOF {
		return nil, ErrCmdBadFormat
	}

	username := msgScanner.TokenText()

	if tok := msgScanner.Scan(); tok == scanner.EOF {
		return nil, ErrCmdBadFormat
	}

	password := msgScanner.TokenText()

	if tok := msgScanner.Scan(); tok != scanner.EOF {
		return nil, ErrCmdBadFormat
	}

	cmd := NewConnectCmd(username, password)

	return cmd, nil
}

func ParseDisconnectCmd(msgScanner *scanner.Scanner) (*DisconnectCmd, error) {
	if tok := msgScanner.Scan(); tok != scanner.EOF {
		return nil, ErrCmdBadFormat
	}

	cmd := NewDisconnectCmd()

	return cmd, nil
}

func ParseSayCmd(msgScanner *scanner.Scanner, msg string) *SayCmd {
	var text string

	if msg == SayCmdStr {
		text = ""
	} else {
		// scanner offset plus the space after the cmd string
		offset := msgScanner.Pos().Offset + 1
		text = msg[offset:]
	}

	cmd := NewSayCmd(text)

	return cmd
}

// MudService

var (
	ErrAuthUserNotFound       = errors.New("auth: user not found")
	ErrAuthInvalidCredentials = errors.New("auth: invalid credentials")
)

type MudService struct {
	userManager *UserManager
}

func NewMudService(userManager *UserManager) *MudService {
	return &MudService{
		userManager: userManager,
	}
}

func (s *MudService) Authenticate(username, password string) (*User, error) {
	user := s.userManager.GetByName(username)

	if user == nil {
		return nil, ErrAuthUserNotFound
	}

	if user.Password() != password {
		return nil, ErrAuthInvalidCredentials
	}

	return user, nil
}

// Client

type Client struct {
	mudService  *MudService
	conn        net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
	connScanner *bufio.Scanner
	msgScanner  *scanner.Scanner
	mux         sync.RWMutex
	manager     *ClientManager
	user        *User
}

func NewClient(mudService *MudService, conn net.Conn) *Client {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	connScanner := bufio.NewScanner(reader)

	var msgScanner scanner.Scanner

	return &Client{
		mudService:  mudService,
		conn:        conn,
		reader:      reader,
		writer:      writer,
		connScanner: connScanner,
		msgScanner:  &msgScanner,
		manager:     nil,
		user:        nil,
	}
}

func (c *Client) SetMudService(service *MudService) {
	c.mudService = service
}

func (c *Client) Conn() net.Conn {
	return c.conn
}

func (c *Client) SetManager(manager *ClientManager) {
	c.manager = manager
}

func (c *Client) SetUser(user *User) {
	c.user = user
}

func (c *Client) Handle() {
	fmt.Fprintf(os.Stdout, "INFO: Connection added (%s).\n", c.conn.RemoteAddr())

	for c.connScanner.Scan() {
		msg := c.connScanner.Text()
		c.HandleUnparsedMessage(msg)
	}

	err := c.connScanner.Err()

	if err == nil {
		c.manager.Remove(c)

		fmt.Fprintf(os.Stdout, "INFO: Connection closed (%s).\n", c.conn.RemoteAddr())

		return
	}

	fmt.Fprintf(os.Stderr, "ERROR: Connection error: %s (%s).\n", c.conn.RemoteAddr(), err)
}

func (c *Client) HandleUnparsedMessage(msg string) {
	c.msgScanner.Init(strings.NewReader(msg))

	if tok := c.msgScanner.Scan(); tok == scanner.EOF {
		return
	}

	cmdstr := c.msgScanner.TokenText()

	if cmdstr == ConnectCmdStr {
		cmd, err := ParseConnectCmd(c.msgScanner)

		if err != nil {
			go c.Write(ConnectCmdUsage())
			return
		}

		c.HandleConnectCmd(cmd)
	} else if cmdstr == DisconnectCmdStr {
		cmd, err := ParseDisconnectCmd(c.msgScanner)

		if err != nil {
			go c.Write(DisconnectCmdUsage())
			return
		}

		c.HandleDisconnectCmd(cmd)
	} else if cmdstr == SayCmdStr {
		cmd := ParseSayCmd(c.msgScanner, msg)

		c.HandleSayCmd(cmd)
	} else {
		cmd := NewUnknownCmd(msg)

		c.HandleUnknownCmd(cmd)
	}
}

func (c *Client) HandleConnectCmd(cmd *ConnectCmd) {
	user, err := c.mudService.Authenticate(cmd.Username(), cmd.Password())

	if err != nil {
		go c.Write("invalid credentials")
		return
	}

	c.SetUser(user)

	go c.Write(fmt.Sprintf("welcome %s", user.Name()))
}

func (c *Client) HandleDisconnectCmd(cmd *DisconnectCmd) {
	if c.user == nil {
		go c.Write("you are not connected")
	} else {
		c.SetUser(nil)

		go c.Write("you were disconnected")
	}
}

func (c *Client) HandleSayCmd(cmd *SayCmd) {
	if len(cmd.Text()) == 0 {
		return
	}

	var username string

	for client := range c.manager.Clients() {
		if client == c {
			continue
		}

		if c.user == nil {
			username = "guest"
		} else {
			username = c.user.Name()
		}

		go client.Write(fmt.Sprintf("<%s>: %s", username, cmd.Text()))
	}
}

func (c *Client) HandleUnknownCmd(cmd *UnknownCmd) {
	go c.Write(fmt.Sprintf("%s: unknown command", cmd.Text()))
}

func (c *Client) Write(msg string) {
	if _, err := fmt.Fprintf(c.conn, "%s\n", msg); err != nil {
		c.manager.Remove(c)
	}
}

// ClientManager

type ClientManager struct {
	mux       sync.RWMutex
	clients   map[*Client]bool
	connIndex map[net.Conn]*Client
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		clients:   make(map[*Client]bool),
		connIndex: make(map[net.Conn]*Client),
	}
}

func (m *ClientManager) Clients() map[*Client]bool {
	return m.clients
}

func (m *ClientManager) Add(client *Client) {
	m.mux.Lock()
	defer m.mux.Unlock()

	client.SetManager(m)

	m.clients[client] = true
	m.connIndex[client.Conn()] = client
}

func (m *ClientManager) Remove(client *Client) {
	m.mux.Lock()
	defer m.mux.Unlock()

	_ = client.Conn().Close()

	client.SetManager(nil)

	delete(m.connIndex, client.Conn())
	delete(m.clients, client)
}

// Server

type Server struct {
	userManager   *UserManager
	clientManager *ClientManager
}

func NewServer(userManager *UserManager) *Server {
	return &Server{
		userManager:   userManager,
		clientManager: NewClientManager(),
	}
}

// Misc

func LoadUsers(userManager *UserManager) {
	userManager.Add(NewUser("user1", "password1"))
	userManager.Add(NewUser("user2", "password2"))
	userManager.Add(NewUser("user3", "password3"))
}

func main() {
	userManager := NewUserManager()

	mudService := NewMudService(userManager)

	LoadUsers(userManager)

	server := NewServer(userManager)

	listener, err := net.Listen("tcp", ":8080")
	defer listener.Close()

	if err != nil {
		panic(err)
	}

	for {
		conn, err := listener.Accept()

		if err != nil {
			panic(err)
		}

		client := NewClient(mudService, conn)

		server.clientManager.Add(client)

		go client.Handle()
	}
}

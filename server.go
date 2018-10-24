package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
)

// ConnManager

type ConnManager struct {
	connMap map[net.Conn]bool
	mux     sync.RWMutex
}

func NewConnManager() *ConnManager {
	return &ConnManager{connMap: make(map[net.Conn]bool)}
}

func (m *ConnManager) add(conn net.Conn) {
	m.mux.Lock()
	m.connMap[conn] = true
	m.mux.Unlock()
}

func (m *ConnManager) remove(conn net.Conn) {
	m.mux.Lock()
	delete(m.connMap, conn)
	m.mux.Unlock()
}

func (m *ConnManager) all() map[net.Conn]bool {
	return m.connMap
}

// MudServer

type MudServer struct {
	ConnManager *ConnManager
}

func NewMudServer() *MudServer {
	return &MudServer{ConnManager: NewConnManager()}
}

func (s *MudServer) HandleConn(conn net.Conn) {
	s.ConnManager.add(conn)

	reader := bufio.NewReader(conn)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		s.HandleMessage(scanner.Text())
	}

	err := scanner.Err()

	if err == nil {
		_ = conn.Close()

		fmt.Println("CONNECTION CLOSED")

		s.ConnManager.remove(conn)
	} else {
		fmt.Fprintln(os.Stderr, "ERROR READING CONNECTION:", err)
	}

	/*
		buf := make([]byte, 1024)

		for {
			nbyte, err := conn.Read(buf)

			if err != nil {
				_ = conn.Close()

				fmt.Println("CONNECTION CLOSED", err)

				s.ConnManager.remove(conn)

				break
			}

			message := make([]byte, nbyte)
			copy(message, buf[:nbyte])

			for conn, _ := range s.ConnManager.all() {
				go s.WriteMessage(conn, message)
			}
		}
	*/
}

func (s *MudServer) HandleMessage(message string) {
	fmt.Println(message)
}

func (s *MudServer) WriteMessage(conn net.Conn, message []byte) {
	total := 0

	for total < len(message) {
		n, err := conn.Write(message[total:])

		if err != nil {
			_ = conn.Close()
			s.ConnManager.remove(conn)
			return
		}

		total += n
	}
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	defer listener.Close()

	if err != nil {
		panic(err)
	}

	server := NewMudServer()

	for {
		conn, err := listener.Accept()

		if err != nil {
			panic(err)
		}

		go server.HandleConn(conn)
	}
}

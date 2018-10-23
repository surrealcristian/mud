package main

import (
	"fmt"
	"net"
)

type Server struct {
	Listener  net.Listener
	NewConns  chan net.Conn
	DeadConns chan net.Conn
}

func NewServer(listener net.Listener, newConnsBufSize int, deadConnsBufSize int) *Server {
	return &Server{
		Listener:  listener,
		NewConns:  make(chan net.Conn, newConnsBufSize),
		DeadConns: make(chan net.Conn, deadConnsBufSize),
	}
}

func (s *Server) HandleAccepts() {
	for {
		conn, err := s.Listener.Accept()

		fmt.Println("ACCEPT")

		if err != nil {
			panic(err)
		}

		s.NewConns <- conn
	}
}

func main() {
	deadConns := make(chan net.Conn, 128)
	publishes := make(chan []byte, 128)
	conns := make(map[net.Conn]bool)

	listener, err := net.Listen("tcp", ":8080")

	server := NewServer(listener, 128)

	if err != nil {
		panic(err)
	}

	go server.HandleAccepts()

	for {
		select {
		case conn := <-server.NewConns:
			conns[conn] = true

			go handleReads(conn, deadConns, publishes)

		case deadConn := <-deadConns:
			_ = deadConn.Close()
			delete(conns, deadConn)

		case publish := <-publishes:
			for conn, _ := range conns {
				handleWrites(conn, deadConns, publish)
			}
		}
	}

	listener.Close()
}

func handleReads(conn net.Conn, deadConns chan<- net.Conn, publishes chan<- []byte) {
	buf := make([]byte, 1024)

	for {
		nbyte, err := conn.Read(buf)

		if err != nil {
			deadConns <- conn
			break
		} else {
			fragment := make([]byte, nbyte)
			copy(fragment, buf[:nbyte])
			publishes <- fragment
		}
	}
}

func handleWrites(conn net.Conn, deadConns chan<- net.Conn, publish []byte) {
	totalWritten := 0

	for totalWritten < len(publish) {
		writtenThisCall, err := conn.Write(publish[totalWritten:])

		if err != nil {
			deadConns <- conn
			break
		}

		totalWritten += writtenThisCall
	}

}

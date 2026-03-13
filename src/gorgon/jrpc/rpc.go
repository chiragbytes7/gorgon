package jrpc

import (
	"errors"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"sync"
	"time"

	"github.com/couchbaselabs/gorgon/src/gorgon/log"
)

type Server struct {
	listener net.Listener
	once     sync.Once
}

func CloseListener() error {
	if rpcServer == nil {
		return nil
	}
	return rpcServer.CloseListener()
}

var rpcServer *Server

func (server *Server) CloseListener() (err error) {
	server.once.Do(func() {
		// Close the Listener if it isnt closed yet
		if server.listener != nil {
			err = server.listener.Close()
		}
	})
	return
}

// Client initiates authenticated connection to RPC server
func Dial(address string, key []byte) (*rpc.Client, error) {
	conn, err := net.DialTimeout("tcp", address, time.Minute)
	if err != nil {
		return nil, err
	}
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	buf := NewBufferedStream(conn)
	err = conn.SetDeadline(time.Now().Add(time.Minute))
	if err != nil {
		return nil, err
	}
	err = newAuthenticator(buf, key).authClient()
	if err != nil {
		return nil, err
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		return nil, err
	}
	conn = nil
	return jsonrpc.NewClient(buf), nil
}

// Server listens for connections; each connection runs in its own goroutine
func Listen(address string, key []byte) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	// Create an instance of the server struct here
	rpcServer = &Server{
		listener: listener,
	}

	defer CloseListener()
	log.Info("RPC listening on %v", listener.Addr())
	for {
		conn, err := listener.Accept()
		// Accept returns with an error when listener.Close() is called on it
		if errors.Is(err, net.ErrClosed) {
			return nil // Not an error if we terminated the server manually
		}
		if err != nil {
			return err
		}
		go handleConnection(conn, key)
	}
}

// HandleConnection authenticates client before serving RPC requests
func handleConnection(conn net.Conn, key []byte) {
	defer conn.Close()
	log.Info("RPC accepted %v", conn.RemoteAddr())
	buf := NewBufferedStream(conn)
	err := conn.SetDeadline(time.Now().Add(time.Minute))
	if err != nil {
		return
	}
	err = newAuthenticator(buf, key).authServer()
	if err != nil {
		log.Warning("RPC auth failed %v %v", conn.RemoteAddr(), err)
		return
	}
	log.Info("RPC auth succeeded %v", conn.RemoteAddr())
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		return
	}
	jsonrpc.ServeConn(buf)
}

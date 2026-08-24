package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

type Server struct {
	HTTP     *http.Server
	Listener net.Listener
}

func Listen(addr string, handler http.Handler) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	return &Server{HTTP: server, Listener: listener}, nil
}
func (s *Server) Address() string { return s.Listener.Addr().String() }
func (s *Server) Serve() error {
	err := s.HTTP.Serve(s.Listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) Shutdown(ctx context.Context) error { return s.HTTP.Shutdown(ctx) }

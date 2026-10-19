package niri

import (
	"bufio"
	"context"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testServer struct {
	t        *testing.T
	listener net.Listener
	path     string
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu       sync.Mutex
	requests []string
}

func startTestServer(t *testing.T, handler func(context.Context, string) string) *testServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "niri.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := &testServer{
		t:        t,
		listener: listener,
		path:     path,
		ctx:      ctx,
		cancel:   cancel,
	}
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			server.wg.Add(1)
			go func() {
				defer server.wg.Done()
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				if !scanner.Scan() {
					return
				}
				request := string(scanner.Bytes())
				server.mu.Lock()
				server.requests = append(server.requests, request)
				server.mu.Unlock()
				reply := handler(ctx, request)
				if reply == "" {
					return
				}
				_, _ = conn.Write([]byte(reply + "\n"))
			}()
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		server.wg.Wait()
	})
	return server
}

func (s *testServer) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *testServer) Config() Config {
	cfg := DefaultConfig(s.path)
	cfg.DialTimeout = 2 * time.Second
	cfg.OpTimeout = 5 * time.Second
	return cfg
}

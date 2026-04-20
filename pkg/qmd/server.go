package qmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ServerConfig struct {
	HTTPAddr        string
	UnixSocket      string
	PersistPath     string
	PersistInterval time.Duration
	MaxWALSize      int
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		HTTPAddr:        ":9876",
		UnixSocket:      "",
		PersistPath:     "",
		PersistInterval: 30 * time.Second,
		MaxWALSize:      100000,
	}
}

type Server struct {
	store    *Store
	config   ServerConfig
	httpSrv  *http.Server
	unixSrv  *http.Server
	mu       sync.Mutex
	running  bool
	httpAddr string
}

func NewServer(cfg ServerConfig) *Server {
	if cfg.HTTPAddr == "" && cfg.UnixSocket == "" {
		cfg.HTTPAddr = ":9876"
	}

	s := &Server{
		store:  NewStore(),
		config: cfg,
	}

	mux := s.newMux()

	if cfg.HTTPAddr != "" {
		s.httpSrv = &http.Server{
			Addr:    cfg.HTTPAddr,
			Handler: mux,
		}
	}

	if cfg.UnixSocket != "" {
		s.unixSrv = &http.Server{
			Handler: mux,
		}
	}

	return s
}

func (s *Server) newMux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/tables", s.handleCreateTable)
	mux.HandleFunc("DELETE /v1/tables/{name}", s.handleDropTable)
	mux.HandleFunc("GET /v1/tables", s.handleListTables)
	mux.HandleFunc("GET /v1/tables/{name}", s.handleGetTable)

	mux.HandleFunc("POST /v1/tables/{table}/records", s.handleInsert)
	mux.HandleFunc("GET /v1/tables/{table}/records/{id}", s.handleGet)
	mux.HandleFunc("PUT /v1/tables/{table}/records/{id}", s.handleUpdate)
	mux.HandleFunc("DELETE /v1/tables/{table}/records/{id}", s.handleDelete)
	mux.HandleFunc("GET /v1/tables/{table}/records", s.handleList)
	mux.HandleFunc("GET /v1/tables/{table}/query", s.handleQuery)
	mux.HandleFunc("GET /v1/tables/{table}/count", s.handleCount)

	mux.HandleFunc("GET /v1/wal", s.handleWAL)
	mux.HandleFunc("POST /v1/wal/truncate", s.handleTruncateWAL)

	mux.HandleFunc("GET /v1/stats", s.handleStats)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/clear", s.handleClear)

	return mux
}

func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("qmd: server already running")
	}

	if s.config.PersistPath != "" {
		if err := s.loadPersist(); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("qmd: load persist: %w", err)
		}
	}

	var httpListener net.Listener
	var unixListener net.Listener
	var err error

	if s.httpSrv != nil {
		httpListener, err = net.Listen("tcp", s.config.HTTPAddr)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("qmd: listen http: %w", err)
		}
		s.httpAddr = httpListener.Addr().String()
	}

	if s.unixSrv != nil {
		os.Remove(s.config.UnixSocket)
		if err := os.MkdirAll(filepath.Dir(s.config.UnixSocket), 0o755); err != nil {
			if httpListener != nil {
				_ = httpListener.Close()
			}
			s.mu.Unlock()
			return fmt.Errorf("qmd: create unix socket dir: %w", err)
		}

		unixListener, err = net.Listen("unix", s.config.UnixSocket)
		if err != nil {
			if httpListener != nil {
				_ = httpListener.Close()
			}
			s.mu.Unlock()
			return fmt.Errorf("qmd: listen unix: %w", err)
		}
	}

	s.running = true
	s.mu.Unlock()

	if s.config.PersistPath != "" {
		go s.persistLoop()
	}

	if httpListener != nil {
		go func() {
			if err := s.httpSrv.Serve(httpListener); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "qmd: http server error: %v\n", err)
			}
		}()
	}

	if unixListener != nil {
		go func() {
			if err := s.unixSrv.Serve(unixListener); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "qmd: unix server error: %v\n", err)
			}
		}()
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.running = false
	s.httpAddr = ""
	s.mu.Unlock()

	if s.config.PersistPath != "" {
		s.persistOnce()
	}

	var errs []error

	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if s.unixSrv != nil {
		if err := s.unixSrv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		os.Remove(s.config.UnixSocket)
	}

	return joinErrs(errs)
}

func (s *Server) Store() *Store {
	return s.store
}

func (s *Server) HTTPBaseURL() string {
	s.mu.Lock()
	addr := s.httpAddr
	configured := s.config.HTTPAddr
	s.mu.Unlock()

	if addr == "" {
		addr = configured
	}
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func (s *Server) persistLoop() {
	ticker := time.NewTicker(s.config.PersistInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.persistOnce()
	}
}

func (s *Server) persistOnce() {
	if s.config.PersistPath == "" {
		return
	}

	data := s.store.WAL()
	if len(data) == 0 {
		return
	}

	tmpPath := s.config.PersistPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return
	}

	enc := json.NewEncoder(f)
	for _, entry := range data {
		if err := enc.Encode(entry); err != nil {
			f.Close()
			return
		}
	}
	f.Close()

	os.Rename(tmpPath, s.config.PersistPath)
	s.store.TruncateWAL()
}

func (s *Server) loadPersist() error {
	if s.config.PersistPath == "" {
		return nil
	}

	f, err := os.Open(s.config.PersistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for dec.More() {
		var entry WALEntry
		if err := dec.Decode(&entry); err != nil {
			break
		}
		s.applyWALEntry(&entry)
	}

	return nil
}

func (s *Server) applyWALEntry(entry *WALEntry) {
	switch entry.Op {
	case "create_table":
		s.store.CreateTable(entry.Table, nil)
	case "drop_table":
		s.store.DropTable(entry.Table)
	case "insert":
		if data, ok := entry.Data.(map[string]any); ok {
			s.store.Insert(entry.Table, &Record{
				ID:        entry.RecordID,
				Table:     entry.Table,
				Data:      data,
				CreatedAt: entry.Timestamp,
				UpdatedAt: entry.Timestamp,
			})
		}
	case "update":
		if data, ok := entry.Data.(map[string]any); ok {
			s.store.Update(entry.Table, &Record{
				ID:        entry.RecordID,
				Table:     entry.Table,
				Data:      data,
				UpdatedAt: entry.Timestamp,
			})
		}
	case "delete":
		s.store.Delete(entry.Table, entry.RecordID)
	}
}

func joinErrs(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("qmd: multiple errors: %v", errs)
}

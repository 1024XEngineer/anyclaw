package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/1024XEngineer/anyclaw/pkg/marketregistry"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8791", "HTTP listen address")
	dataDir := fs.String("data-dir", ".anyclaw-registry", "registry data directory")
	dbDriver := fs.String("db-driver", "sqlite", "database/sql driver name")
	dbDSN := fs.String("db-dsn", "", "database DSN; defaults to <data-dir>/registry.db for sqlite")
	adminToken := fs.String("admin-token", os.Getenv("ANYCLAW_REGISTRY_ADMIN_TOKEN"), "admin bearer token; defaults to ANYCLAW_REGISTRY_ADMIN_TOKEN")
	requireAdminToken := fs.Bool("require-admin-token", envBool("ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN", true), "fail startup when admin token is empty")
	seed := fs.Bool("seed", true, "seed fixture artifacts when the registry is empty")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, err := marketregistry.NewServer(ctx, marketregistry.ServerConfig{
		Addr:              *addr,
		DataDir:           *dataDir,
		DBDriver:          *dbDriver,
		DBDSN:             *dbDSN,
		AdminToken:        *adminToken,
		RequireAdminToken: *requireAdminToken,
		Seed:              *seed,
	})
	if err != nil {
		return err
	}
	defer server.Close()

	log.Printf("anyclaw registry listening on %s, data_dir=%s", *addr, *dataDir)
	err = server.StartWithContext(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func printUsage() {
	fmt.Println("Usage: anyclaw-registry serve [--addr :8791] [--data-dir .anyclaw-registry] [--db-driver sqlite] [--db-dsn path-or-url] [--admin-token token] [--require-admin-token=true] [--seed=true]")
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

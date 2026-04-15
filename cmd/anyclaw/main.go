package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"anyclaw/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, "")
	if err != nil {
		log.Fatalf("build app: %v", err)
	}
	if err := application.Start(ctx); err != nil {
		log.Fatalf("start app: %v", err)
	}
	<-ctx.Done()
	if err := application.Shutdown(context.Background()); err != nil {
		log.Fatalf("shutdown app: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"wtt/internal/client"
	"wtt/internal/server"
	"wtt/internal/ui"

	"github.com/alecthomas/kong"
)

func main() {
	k := kong.Parse(&ui.Cli)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch k.Command() {
	case "service":
		cfg := client.ServiceConfig{
			ID:       ui.Cli.Service.ID,
			Type:     ui.Cli.Service.Type,
			Output:   ui.Cli.Service.Output,
			Endpoint: ui.Cli.Service.Endpoint,
		}
		s, err := client.NewService(cfg)
		if err != nil {
			log.Fatalf("failed to create service: %v", err)
		}
		if err := s.Register(ctx); err != nil {
			log.Fatalf("failed to register service: %v", err)
		}
		s.Serve(ctx)
	case "consumer":
		cfg := client.ConsumerConfig{
			ID:       ui.Cli.Consumer.ID,
			Type:     ui.Cli.Consumer.Type,
			Input:    ui.Cli.Consumer.Input,
			Endpoint: ui.Cli.Consumer.Endpoint,
		}
		c, err := client.NewConsumer(cfg)
		if err != nil {
			log.Fatalf("failed to create consumer: %v", err)
		}
		if err := c.Register(ctx); err != nil {
			log.Fatalf("failed to register consumer: %v", err)
		}
		if err := c.Connect(ctx, ui.Cli.Consumer.ServiceID); err != nil {
			log.Fatalf("failed to connect to service: %v", err)
		}
		c.Serve(ctx)
	case "server":
		cfg := server.ServerConfig{
			Endpoint: ui.Cli.Server.Endpoint,
		}
		s, err := server.NewServer(cfg)
		if err != nil {
			log.Fatalf("failed to create server: %v", err)
		}

		go func() {
			for err := range s.ErrorChannel {
				log.Printf("server error: %v", err)
			}
		}()

		if err := s.Start(ctx); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
		log.Printf("server started on %s", ui.Cli.Server.Endpoint)

		<-ctx.Done()

		if err := s.Shutdown(context.Background()); err != nil {
			log.Printf("failed to shutdown server: %v", err)
		}
	default:
		panic(k.Command())
	}
}

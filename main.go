package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"wtt/internal/client"
	"wtt/internal/log"
	"wtt/internal/server"
	"wtt/internal/typ"
	"wtt/internal/ui"

	"github.com/alecthomas/kong"
)

func main() {
	k := kong.Parse(&ui.Cli)
	log.Init(ui.Cli.LogLevel)

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
		s.HandleOffer = func(offer *typ.RTCOffer) (reason string) {
			return ""
		}
		if err != nil {
			slog.Error("failed to create service", "err", err)
			os.Exit(1)
		}
		if err := s.Register(ctx); err != nil {
			slog.Error("failed to register service", "err", err)
			os.Exit(1)
		}
		go s.Serve(ctx)

		<-ctx.Done()
		s.Shutdown(context.Background())
	case "consumer":
		cfg := client.ConsumerConfig{
			ID:       ui.Cli.Consumer.ID,
			Type:     ui.Cli.Consumer.Type,
			Input:    ui.Cli.Consumer.Input,
			Endpoint: ui.Cli.Consumer.Endpoint,
		}
		c, err := client.NewConsumer(cfg)
		if err != nil {
			slog.Error("failed to create consumer", "err", err)
			os.Exit(1)
		}
		if err := c.Register(ctx); err != nil {
			slog.Error("failed to register consumer", "err", err)
			os.Exit(1)
		}
		if err := c.Connect(ctx, ui.Cli.Consumer.ServiceID); err != nil {
			slog.Error("failed to connect to service", "err", err)
			os.Exit(1)
		}
		go c.Serve(ctx)

		<-ctx.Done()
		c.Shutdown(context.Background())
	case "server":
		cfg := server.ServerConfig{
			Endpoint: ui.Cli.Server.Endpoint,
		}
		s, err := server.NewServer(cfg)
		if err != nil {
			slog.Error("failed to create server", "err", err)
			os.Exit(1)
		}

		go func() {
			for err := range s.ErrorChannel {
				slog.Error("server error", "err", err)
			}
		}()

		if err := s.Start(ctx); err != nil {
			slog.Error("failed to start server", "err", err)
			os.Exit(1)
		}
		slog.Info("server started", "endpoint", ui.Cli.Server.Endpoint)

		<-ctx.Done()

		if err := s.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown server", "err", err)
		}
	default:
		panic(k.Command())
	}
}

package client

import "context"

type ConsumerConfig struct {
	ID string
}

type ConsumerState struct{}

type Consumer struct {
	*ConsumerConfig
	*ConsumerState

	ErrorChannel chan error

	ConsumerInterface
}

type ConsumerInterface interface {
	Register(ctx context.Context) error

	Connect(ctx context.Context, sid string) error
	Close(ctx context.Context) error
}

// TODO

var _ ConsumerInterface = (*Consumer)(nil)

package messaging

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Connection holds a NATS connection and publisher used to dispatch agent creates.
type Connection struct {
	Publisher *Publisher
	JetStream jetstream.JetStream

	nc *nats.Conn
}

// Connect dials NATS, creates JetStream, and ensures the agent request stream exists.
func Connect(ctx context.Context, natsURL string) (*Connection, error) {
	nc, err := nats.Connect(natsURL, nats.MaxReconnects(-1))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream: %w", err)
	}

	publisher := NewPublisher(js)
	if err := publisher.EnsureStream(ctx); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensure agent request stream: %w", err)
	}

	return &Connection{
		Publisher: publisher,
		JetStream: js,
		nc:        nc,
	}, nil
}

// Close closes the NATS connection.
func (c *Connection) Close() {
	if c == nil {
		return
	}
	if c.nc != nil {
		c.nc.Close()
	}
}

// Package consumer subscribes to NATS JetStream status events.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2/event"
	placementservice "github.com/dcm-project/control-plane/internal/placement/service"
	placementtypes "github.com/dcm-project/control-plane/internal/placement/types"
	"github.com/dcm-project/control-plane/internal/sp/store"
	rmstore "github.com/dcm-project/control-plane/internal/sp/store/resource_manager"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const healthFlushTimeout = 2 * time.Second

// defaultStatusMaxDeliver caps redelivery when placement callbacks keep failing.
const defaultStatusMaxDeliver = 10

// StatusEvent represents a status event payload.
type StatusEvent struct {
	Id         string         `json:"id"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	OutputSpec map[string]any `json:"output_spec,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// StatusConsumer subscribes to status events from NATS JetStream
// and updates ServiceTypeInstance records in the database.
type StatusConsumer struct {
	conn         *nats.Conn
	js           jetstream.JetStream
	consumeCtx   jetstream.ConsumeContext
	store        store.Store
	subject      string
	streamName   string
	consumerName string
	onRunning    func(context.Context, placementtypes.ResourceStatusEvent) error
	onDeleted    func(context.Context, string) error
	onFailed     func(context.Context, string) error
}

// New creates a new StatusConsumer connected to the given NATS URL.
func New(natsURL, subject string, st store.Store, opts ...Option) (*StatusConsumer, error) {
	conn, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	sc := &StatusConsumer{
		conn:         conn,
		js:           js,
		store:        st,
		subject:      subject,
		streamName:   "dcm-status",
		consumerName: "service-provider-manager",
	}
	for _, o := range opts {
		o(sc)
	}
	return sc, nil
}

// Option configures a StatusConsumer.
type Option func(*StatusConsumer)

// SetStreamName sets the JetStream stream name.
func SetStreamName(name string) Option {
	return func(c *StatusConsumer) { c.streamName = name }
}

// SetConsumerName sets the JetStream durable consumer name.
func SetConsumerName(name string) Option {
	return func(c *StatusConsumer) { c.consumerName = name }
}

// SetPlacementStatusHandlers registers placement callbacks for async DAG orchestration.
func SetPlacementStatusHandlers(
	onRunning func(context.Context, placementtypes.ResourceStatusEvent) error,
	onDeleted func(context.Context, string) error,
	onFailed func(context.Context, string) error,
) Option {
	return func(c *StatusConsumer) {
		c.onRunning = onRunning
		c.onDeleted = onDeleted
		c.onFailed = onFailed
	}
}

// Start creates the JetStream stream and consumer, then begins processing messages.
func (c *StatusConsumer) Start(ctx context.Context) error {
	stream, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     c.streamName,
		Subjects: []string{c.subject},
	})
	if err != nil {
		return fmt.Errorf("failed to create/update stream %s: %w", c.streamName, err)
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:    c.consumerName,
		AckPolicy:  jetstream.AckExplicitPolicy,
		MaxDeliver: defaultStatusMaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("failed to create/update consumer %s: %w", c.consumerName, err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		c.handleMessage(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}
	c.consumeCtx = cc

	slog.Info("StatusConsumer subscribed",
		"subject", c.subject,
		"stream", c.streamName,
		"consumer", c.consumerName,
	)
	return nil
}

// Stop stops the consumer and closes the NATS connection.
func (c *StatusConsumer) Stop() {
	if c.consumeCtx != nil {
		c.consumeCtx.Stop()
	}
	c.conn.Close()
	slog.Info("StatusConsumer stopped")
}

// Check verifies the NATS connection is usable (connected and responsive).
func (c *StatusConsumer) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.conn == nil || !c.conn.IsConnected() {
		return errors.New("nats not connected")
	}

	timeout := healthFlushTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			return context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}

	if err := c.conn.FlushTimeout(timeout); err != nil {
		return fmt.Errorf("nats flush: %w", err)
	}
	return nil
}

func (c *StatusConsumer) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var event cloudevents.Event
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.Error("Failed to parse CloudEvent", "error", err)
		_ = msg.Ack()
		return
	}

	var payload StatusEvent
	if err := json.Unmarshal(event.Data(), &payload); err != nil {
		slog.Error("Failed to deserialize event payload", "error", err)
		_ = msg.Ack()
		return
	}

	if payload.Id == "" {
		slog.Warn("Event missing instance ID, discarding")
		_ = msg.Ack()
		return
	}

	// This event's status string comes from an external CloudEvent producer
	// we don't control, so it's normalized to the lowercase convention used
	// throughout the rest of the status lifecycle before it crosses into our
	// store, rather than trusting upstream casing (see internal/sp/store/model/status.go).
	normalizedStatus := strings.ToLower(strings.TrimSpace(payload.Status))

	if err := c.store.ServiceTypeInstance().UpdateStatus(ctx, payload.Id, normalizedStatus, payload.Message, payload.OutputSpec); err != nil {
		if errors.Is(err, rmstore.ErrInstanceNotFound) {
			slog.Warn("No instance found, skipping status update", "instance_id", payload.Id)
			_ = msg.Ack()
			return
		}
		slog.Error("Failed to update instance status", "instance_id", payload.Id, "error", err)
		_ = msg.Nak()
		return
	}

	if fields := sortedOutputFields(payload.OutputSpec); len(fields) > 0 {
		slog.Info("audit: output_spec persisted from status event",
			"instance_id", payload.Id,
			"status", normalizedStatus,
			"output_fields", fields,
		)
	}

	slog.Info("Instance status updated", "instance_id", payload.Id, "status", normalizedStatus)

	switch strings.ToUpper(normalizedStatus) {
	case placementtypes.ResourceStatusRunning:
		if c.onRunning != nil {
			if err := c.onRunning(ctx, placementtypes.ResourceStatusEvent{
				ResourceID: payload.Id,
				Status:     normalizedStatus,
				OutputSpec: payload.OutputSpec,
			}); err != nil {
				if placementservice.IsCallbackRetryable(err) {
					slog.Warn("Placement OnResourceRunning failed with retryable error, nacking",
						"instance_id", payload.Id, "error", err)
					_ = msg.NakWithDelay(5 * time.Second)
					return
				}
				slog.Error("Placement OnResourceRunning failed", "instance_id", payload.Id, "error", err)
			}
		}
	case placementtypes.ResourceStatusDeleted:
		if c.onDeleted != nil {
			if err := c.onDeleted(ctx, payload.Id); err != nil {
				if placementservice.IsCallbackRetryable(err) {
					slog.Warn("Placement OnResourceDeleted failed with retryable error, nacking",
						"instance_id", payload.Id, "error", err)
					_ = msg.NakWithDelay(5 * time.Second)
					return
				}
				slog.Error("Placement OnResourceDeleted failed", "instance_id", payload.Id, "error", err)
			}
		}
	case placementtypes.ResourceStatusFailed:
		if c.onFailed != nil {
			if err := c.onFailed(ctx, payload.Id); err != nil {
				if placementservice.IsCallbackRetryable(err) {
					slog.Warn("Placement OnResourceFailed failed with retryable error, nacking",
						"instance_id", payload.Id, "error", err)
					_ = msg.NakWithDelay(5 * time.Second)
					return
				}
				slog.Error("Placement OnResourceFailed failed", "instance_id", payload.Id, "error", err)
			}
		}
	}
	_ = msg.Ack()
}

func sortedOutputFields(outputSpec map[string]any) []string {
	if len(outputSpec) == 0 {
		return nil
	}
	fields := make([]string, 0, len(outputSpec))
	for key := range outputSpec {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

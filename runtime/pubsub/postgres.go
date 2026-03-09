package pubsub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lib/pq"
)

// PostgresBroker wraps an InProcessBroker with a Postgres LISTEN goroutine.
// Publish issues pg_notify so subscribers in this process and any peer process
// listening on the same channel receive the payload; the in-process broker
// fans it out to local subscribers the moment NOTIFY arrives.
type PostgresBroker struct {
	local    *InProcessBroker
	listener *pq.Listener
	dsn      string

	channels map[string]struct{}

	closed chan struct{}
	once   sync.Once
}

// NewPostgresBroker connects a pq.Listener to dsn and returns a broker that
// proxies Publish to pg_notify and forwards inbound NOTIFY payloads to the
// in-process broker. Pass 0 for minReconnect/maxReconnect to use 10s/1m.
func NewPostgresBroker(ctx context.Context, dsn string, channels []string, minReconnect, maxReconnect time.Duration) (*PostgresBroker, error) {
	if dsn == "" {
		return nil, errors.New("pubsub: dsn required")
	}
	if minReconnect == 0 {
		minReconnect = 10 * time.Second
	}
	if maxReconnect == 0 {
		maxReconnect = time.Minute
	}

	pb := &PostgresBroker{
		local:    NewInProcessBroker(),
		dsn:      dsn,
		channels: make(map[string]struct{}, len(channels)),
		closed:   make(chan struct{}),
	}

	listener := pq.NewListener(dsn, minReconnect, maxReconnect, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("PUBSUB001: listener event=%d err=%v", ev, err)
		}
	})
	pb.listener = listener

	for _, ch := range channels {
		if err := listener.Listen(ch); err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf("pubsub: listen %s: %w", ch, err)
		}
		pb.channels[ch] = struct{}{}
	}

	go pb.run(ctx)
	return pb, nil
}

// run pumps notifications from the pq.Listener into the in-process broker until
// the broker is closed or ctx is cancelled.
func (b *PostgresBroker) run(ctx context.Context) {
	for {
		select {
		case <-b.closed:
			return
		case <-ctx.Done():
			_ = b.Close()
			return
		case n := <-b.listener.Notify:
			if n == nil {
				// nil notification signals reconnect; pq.Listener handles
				// re-subscribing to channels we already registered.
				continue
			}
			_ = b.local.Publish(ctx, n.Channel, []byte(n.Extra))
		case <-time.After(90 * time.Second):
			// Periodic ping keeps the TCP connection alive across NAT
			// timeouts. Best-effort — pq.Listener reconnects on failure.
			go func() {
				if err := b.listener.Ping(); err != nil {
					log.Printf("PUBSUB002: listener ping err=%v", err)
				}
			}()
		}
	}
}

// Publish issues pg_notify on the channel matching topic. Local fan-out happens
// through the listener's NOTIFY callback so this does not double-deliver.
func (b *PostgresBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	if b.local == nil {
		return errors.New("pubsub: broker closed")
	}

	// Short-lived connection rather than a persistent publish connection —
	// publishes are infrequent compared to the listen loop.
	conn, err := sql.Open("postgres", b.dsn)
	if err != nil {
		return fmt.Errorf("pubsub: open: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_notify($1, $2)", topic, string(payload)); err != nil {
		return fmt.Errorf("pubsub: pg_notify %s: %w", topic, err)
	}
	return nil
}

// Subscribe delegates to the in-process broker so callers receive every payload
// that arrived via NOTIFY (or via a local Publish that round-trips through it).
func (b *PostgresBroker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	return b.local.Subscribe(ctx, topic)
}

// Close shuts the listener down and closes every local subscriber channel.
func (b *PostgresBroker) Close() error {
	var err error
	b.once.Do(func() {
		close(b.closed)
		if b.listener != nil {
			err = b.listener.Close()
		}
		if b.local != nil {
			_ = b.local.Close()
		}
	})
	return err
}

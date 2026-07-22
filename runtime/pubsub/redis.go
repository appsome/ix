package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisOptions configure NewRedisBroker.
type RedisOptions struct {
	// URL is the Redis to publish to and listen on: redis://[:password@]host:port[/db],
	// or rediss:// for TLS. Required.
	URL string
	// Prefix namespaces the Redis channels: topic t is published on Prefix+t,
	// so deployments sharing one Redis do not hear each other. It may not
	// hold the glob characters * ? [ ], which the listening pattern would
	// read as wildcards.
	Prefix string
	// PingInterval is how often the broker checks Redis to keep
	// pubsub_connected true and to log a lost or regained connection.
	// 0 uses 15s.
	PingInterval time.Duration
}

// RedisBroker wraps an InProcessBroker with a Redis PSUBSCRIBE connection.
// Publish issues PUBLISH on Prefix+topic, so subscribers in this process and
// in every peer process listening on the same Redis receive the payload; the
// in-process broker fans it out to local subscribers when the message comes
// back on the pattern, so a local subscriber never gets it twice.
//
// Delivery is at most once: Redis keeps nothing, and a payload published
// while a listener is disconnected is lost to it. Subscribers treat a payload
// as a hint and read the state it names from its owner.
type RedisBroker struct {
	local  *InProcessBroker
	client *redis.Client
	sub    *redis.PubSub
	prefix string

	closed chan struct{}
	once   sync.Once
}

// NewRedisBroker connects to opts.URL, subscribes to every channel under
// opts.Prefix and returns once Redis has confirmed the subscription, so a
// Redis that cannot be reached fails the caller's startup rather than its
// first event. The broker runs until ctx is cancelled or Close is called.
func NewRedisBroker(ctx context.Context, opts RedisOptions) (*RedisBroker, error) {
	if opts.URL == "" {
		return nil, errors.New("pubsub: redis url required")
	}
	if strings.ContainsAny(opts.Prefix, "*?[]") {
		return nil, fmt.Errorf("pubsub: redis prefix %q holds a glob character (* ? [ ])", opts.Prefix)
	}
	options, err := redis.ParseURL(opts.URL)
	if err != nil {
		// The URL may hold a password: the error names neither.
		return nil, fmt.Errorf("pubsub: redis url: %w", err)
	}
	interval := opts.PingInterval
	if interval == 0 {
		interval = 15 * time.Second
	}

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pubsub: redis ping %s: %w", options.Addr, err)
	}
	sub := client.PSubscribe(ctx, opts.Prefix+"*")
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		_ = client.Close()
		return nil, fmt.Errorf("pubsub: redis psubscribe %s*: %w", opts.Prefix, err)
	}
	connected.Set(1)

	b := &RedisBroker{
		local:  NewInProcessBroker(),
		client: client,
		sub:    sub,
		prefix: opts.Prefix,
		closed: make(chan struct{}),
	}
	go b.run(ctx)
	go b.watch(ctx, interval)
	return b, nil
}

// run hands every message on the pattern to the local subscribers of its
// topic. go-redis reconnects and subscribes again on its own.
func (b *RedisBroker) run(ctx context.Context) {
	messages := b.sub.Channel()
	for {
		select {
		case <-b.closed:
			return
		case <-ctx.Done():
			_ = b.Close()
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			topic := strings.TrimPrefix(message.Channel, b.prefix)
			received.WithLabelValues(topic).Inc()
			b.local.deliver(topic, []byte(message.Payload))
		}
	}
}

// watch pings Redis every interval: pubsub_connected follows the answer, and
// a lost or regained connection is logged once (PUBSUB003, PUBSUB004).
func (b *RedisBroker) watch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	up := true
	for {
		select {
		case <-b.closed:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := b.Ping(ctx)
			switch {
			case err != nil && up:
				up = false
				connected.Set(0)
				log.Printf("PUBSUB003: redis unreachable, payloads published until it is back are lost: %v", err)
			case err == nil && !up:
				up = true
				connected.Set(1)
				log.Printf("PUBSUB004: redis reachable again")
			}
		}
	}
}

// Ping checks that Redis answers, for a readiness probe.
func (b *RedisBroker) Ping(ctx context.Context) error {
	if err := b.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("pubsub: redis ping: %w", err)
	}
	return nil
}

// Publish issues PUBLISH on the topic's channel. The payload reaches local
// subscribers through the pattern subscription, like any peer's.
func (b *RedisBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	select {
	case <-b.closed:
		return errors.New("pubsub: broker closed")
	default:
	}
	if err := b.client.Publish(ctx, b.prefix+topic, payload).Err(); err != nil {
		return fmt.Errorf("pubsub: redis publish %s: %w", topic, err)
	}
	published.WithLabelValues(topic).Inc()
	return nil
}

// Subscribe delegates to the in-process broker, which receives every payload
// that arrives on the pattern.
func (b *RedisBroker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	return b.local.Subscribe(ctx, topic)
}

// Close unsubscribes, closes the Redis connections and every local
// subscriber channel. Safe to call more than once.
func (b *RedisBroker) Close() error {
	var err error
	b.once.Do(func() {
		close(b.closed)
		connected.Set(0)
		err = errors.Join(b.sub.Close(), b.client.Close())
		_ = b.local.Close()
	})
	return err
}

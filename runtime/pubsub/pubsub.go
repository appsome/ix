// Package pubsub provides an in-process publish/subscribe broker plus
// optional Postgres LISTEN/NOTIFY and Redis drivers. It backs gqlgen GraphQL
// subscriptions and lets sibling processes reach each other's subscribers.
//
// Callers Publish a payload to a topic and Subscribe to receive a fan-out copy
// of every payload published on that topic until the supplied context is
// cancelled. The broker is goroutine-safe; a slow subscriber cannot block
// others — payloads destined for a full subscriber channel are dropped after a
// non-blocking attempt and counted (pubsub_dropped_total).
//
// No domain-specific topic constants are defined here: topics are plain strings
// the project owns.
package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
)

// Broker is the surface a Subscription resolver depends on. The in-process
// implementation is the default; PostgresBroker and RedisBroker compose it
// with a listening goroutine so publishers in sibling processes reach
// subscribers via NOTIFY or Redis PUBLISH.
type Broker interface {
	// Publish fans payload out to every active subscriber on topic.
	Publish(ctx context.Context, topic string, payload []byte) error
	// Subscribe returns a buffered channel emitting one entry per payload
	// published on topic. The channel closes when ctx is cancelled or the
	// broker shuts down. A backed-up subscriber drops new payloads rather
	// than blocking the publisher.
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
	// Close releases broker resources. Safe to call multiple times.
	Close() error
}

// defaultSubscriberBuffer is the per-subscriber channel size — large enough to
// absorb a burst without spilling, bounded for many concurrent subscribers.
const defaultSubscriberBuffer = 64

// InProcessBroker fans payloads out to in-memory subscribers. Publish never
// blocks: when a subscriber's channel is full the payload is dropped for that
// subscriber only.
type InProcessBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[*subscriber]struct{}
	closed      atomic.Bool
}

type subscriber struct {
	ch chan []byte
}

// NewInProcessBroker constructs an empty broker.
func NewInProcessBroker() *InProcessBroker {
	return &InProcessBroker{
		subscribers: make(map[string]map[*subscriber]struct{}),
	}
}

// Publish forwards payload to every subscriber on topic. Always returns nil;
// the error in the signature keeps PostgresBroker shape-compatible.
func (b *InProcessBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	if b.closed.Load() {
		return nil
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	published.WithLabelValues(topic).Inc()
	b.deliver(topic, payload)
	return nil
}

// deliver hands payload to every local subscriber on topic: Publish's fan-out,
// and the path a payload from Redis or Postgres takes, which is counted as
// received rather than published.
func (b *InProcessBroker) deliver(topic string, payload []byte) {
	if b.closed.Load() {
		return
	}
	b.mu.RLock()
	subs := b.subscribers[topic]
	// Snapshot the subscriber set so we can release the read lock before
	// pushing to channels — otherwise a slow consumer would hold every other
	// publisher off the topic.
	snap := make([]*subscriber, 0, len(subs))
	for s := range subs {
		snap = append(snap, s)
	}
	b.mu.RUnlock()

	for _, s := range snap {
		select {
		case s.ch <- payload:
		default:
			// Drop for this subscriber rather than blocking publishers,
			// counted so a subscriber that falls behind is visible.
			dropped.WithLabelValues(topic).Inc()
		}
	}
}

// Subscribe registers a new subscriber on topic and returns its receive
// channel. The channel closes when ctx is cancelled or Close is called.
func (b *InProcessBroker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	if b.closed.Load() {
		ch := make(chan []byte)
		close(ch)
		return ch, nil
	}

	s := &subscriber{ch: make(chan []byte, defaultSubscriberBuffer)}

	b.mu.Lock()
	if _, ok := b.subscribers[topic]; !ok {
		b.subscribers[topic] = make(map[*subscriber]struct{})
	}
	b.subscribers[topic][s] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.unsubscribe(topic, s)
	}()

	return s.ch, nil
}

func (b *InProcessBroker) unsubscribe(topic string, s *subscriber) {
	b.mu.Lock()
	if subs, ok := b.subscribers[topic]; ok {
		if _, present := subs[s]; present {
			delete(subs, s)
			close(s.ch)
		}
		if len(subs) == 0 {
			delete(b.subscribers, topic)
		}
	}
	b.mu.Unlock()
}

// Close marks the broker shut and closes every active subscriber channel.
// Subsequent Publish calls return nil and Subscribe returns a closed channel.
func (b *InProcessBroker) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	b.mu.Lock()
	for topic, subs := range b.subscribers {
		for s := range subs {
			close(s.ch)
		}
		delete(b.subscribers, topic)
	}
	b.mu.Unlock()
	return nil
}

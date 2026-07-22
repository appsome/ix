package pubsub

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func redisBroker(t *testing.T, server *miniredis.Miniredis, prefix string) *RedisBroker {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b, err := NewRedisBroker(ctx, RedisOptions{URL: "redis://" + server.Addr(), Prefix: prefix, PingInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewRedisBroker: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func subscribe(t *testing.T, b Broker, topic string) <-chan []byte {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, err := b.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe %s: %v", topic, err)
	}
	return ch
}

func expect(t *testing.T, ch <-chan []byte, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no payload, want %q", want)
	}
}

func expectNothing(t *testing.T, ch <-chan []byte) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("got %q, want nothing", got)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestRedisBroker_APublishReachesEveryProcessOnce: two brokers stand for two
// processes on one Redis; each subscriber gets the payload exactly once,
// the publisher's own included.
func TestRedisBroker_APublishReachesEveryProcessOnce(t *testing.T) {
	server := miniredis.RunT(t)
	a, b := redisBroker(t, server, "app:"), redisBroker(t, server, "app:")
	onA, onB := subscribe(t, a, topicA), subscribe(t, b, topicA)

	if err := a.Publish(context.Background(), topicA, []byte(`{"id":1}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	expect(t, onA, `{"id":1}`)
	expect(t, onB, `{"id":1}`)
	expectNothing(t, onA)
	expectNothing(t, onB)
}

func TestRedisBroker_ATopicReachesOnlyItsSubscribers(t *testing.T) {
	server := miniredis.RunT(t)
	a, b := redisBroker(t, server, "app:"), redisBroker(t, server, "app:")
	onB := subscribe(t, b, topicB)

	if err := a.Publish(context.Background(), topicA, []byte("a")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	expectNothing(t, onB)
}

// TestRedisBroker_DeploymentsSharingARedisDoNotHearEachOther: the prefix
// namespaces the channels.
func TestRedisBroker_DeploymentsSharingARedisDoNotHearEachOther(t *testing.T) {
	server := miniredis.RunT(t)
	staging, production := redisBroker(t, server, "staging:"), redisBroker(t, server, "production:")
	onProduction := subscribe(t, production, topicA)

	if err := staging.Publish(context.Background(), topicA, []byte("staging")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	expectNothing(t, onProduction)
}

// TestRedisBroker_DeliversAgainAfterRedisComesBack: go-redis subscribes again
// on its own; pubsub_connected follows Redis down and back up.
func TestRedisBroker_DeliversAgainAfterRedisComesBack(t *testing.T) {
	server := miniredis.RunT(t)
	a, b := redisBroker(t, server, "app:"), redisBroker(t, server, "app:")
	onB := subscribe(t, b, topicA)

	server.Close()
	waitFor(t, "pubsub_connected 0 while Redis is down", func() bool { return testutil.ToFloat64(connected) == 0 })
	if err := server.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	waitFor(t, "pubsub_connected 1 once Redis is back", func() bool { return testutil.ToFloat64(connected) == 1 })

	// The listener resubscribes a moment after the connection returns: the
	// payload is published until it arrives, as at-most-once delivery allows.
	deadline := time.After(5 * time.Second)
	for {
		if err := a.Publish(context.Background(), topicA, []byte("again")); err != nil {
			t.Logf("Publish while reconnecting: %v", err)
		}
		select {
		case got := <-onB:
			if string(got) != "again" {
				t.Fatalf("got %q", got)
			}
			return
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatal("no payload after Redis came back")
		}
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRedisBroker_AnUnreachableRedisFailsTheConstructor: a broken Redis is a
// startup failure, not a silent first lost event.
func TestRedisBroker_AnUnreachableRedisFailsTheConstructor(t *testing.T) {
	server := miniredis.RunT(t)
	addr := server.Addr()
	server.Close()

	_, err := NewRedisBroker(context.Background(), RedisOptions{URL: "redis://" + addr})

	if err == nil || !strings.Contains(err.Error(), "redis ping") {
		t.Fatalf("err = %v, want a redis ping failure", err)
	}
}

func TestRedisBroker_RefusesWhatItCannotUse(t *testing.T) {
	for name, opts := range map[string]RedisOptions{
		"no url":             {},
		"not a redis url":    {URL: "http://example"},
		"a glob in a prefix": {URL: "redis://127.0.0.1:1", Prefix: "app*"},
	} {
		if _, err := NewRedisBroker(context.Background(), opts); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestRedisBroker_TheURLsPasswordStaysOutOfItsErrors: the URL is a secret.
func TestRedisBroker_TheURLsPasswordStaysOutOfItsErrors(t *testing.T) {
	_, err := NewRedisBroker(context.Background(), RedisOptions{URL: "redis://:s3cret@127.0.0.1:1"})

	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v, want an error without the password", err)
	}
}

func TestRedisBroker_CloseClosesSubscribersAndRefusesPublish(t *testing.T) {
	server := miniredis.RunT(t)
	b := redisBroker(t, server, "app:")
	ch := subscribe(t, b, topicA)

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if _, open := <-ch; open {
		t.Fatal("subscriber channel still open")
	}
	if err := b.Publish(context.Background(), topicA, []byte("x")); err == nil {
		t.Fatal("Publish after Close: no error")
	}
}

// TestInProcessBroker_AFullSubscriberIsCounted: a dropped payload was
// silent; pubsub_dropped_total shows a subscriber that falls behind.
func TestInProcessBroker_AFullSubscriberIsCounted(t *testing.T) {
	const topic = "dropped_topic"
	b := NewInProcessBroker()
	defer b.Close()
	subscribe(t, b, topic) // never read
	before := testutil.ToFloat64(dropped.WithLabelValues(topic))

	for i := 0; i < defaultSubscriberBuffer+3; i++ {
		if err := b.Publish(context.Background(), topic, []byte("x")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	if got := testutil.ToFloat64(dropped.WithLabelValues(topic)) - before; got != 3 {
		t.Fatalf("dropped %v, want 3", got)
	}
}

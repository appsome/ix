package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"
)

const (
	topicA = "topic_a"
	topicB = "topic_b"
)

// TestInProcessBroker_FanOut publishes one payload and expects every subscriber
// on the topic to observe it.
func TestInProcessBroker_FanOut(t *testing.T) {
	t.Parallel()

	b := NewInProcessBroker()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const subs = 5
	chans := make([]<-chan []byte, subs)
	for i := 0; i < subs; i++ {
		ch, err := b.Subscribe(ctx, topicA)
		if err != nil {
			t.Fatalf("Subscribe[%d]: %v", i, err)
		}
		chans[i] = ch
	}

	payload := []byte(`{"id":1}`)
	if err := b.Publish(ctx, topicA, payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range chans {
		select {
		case got := <-ch:
			if string(got) != string(payload) {
				t.Errorf("subscriber[%d] got %q, want %q", i, got, payload)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber[%d] did not receive payload", i)
		}
	}
}

// TestInProcessBroker_TopicIsolation verifies subscribers on one topic don't
// receive payloads published on a different topic.
func TestInProcessBroker_TopicIsolation(t *testing.T) {
	t.Parallel()

	b := NewInProcessBroker()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aCh, err := b.Subscribe(ctx, topicA)
	if err != nil {
		t.Fatalf("Subscribe a: %v", err)
	}
	bCh, err := b.Subscribe(ctx, topicB)
	if err != nil {
		t.Fatalf("Subscribe b: %v", err)
	}

	if err := b.Publish(ctx, topicB, []byte("cmd")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-bCh:
		if string(got) != "cmd" {
			t.Errorf("topicB subscriber got %q, want %q", got, "cmd")
		}
	case <-time.After(time.Second):
		t.Fatal("topicB subscriber did not receive payload")
	}

	select {
	case got := <-aCh:
		t.Errorf("topicA subscriber got payload from topicB: %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestInProcessBroker_Unsubscribe ensures cancelling the subscriber's context
// closes the channel and removes the subscriber from the topic map.
func TestInProcessBroker_Unsubscribe(t *testing.T) {
	t.Parallel()

	b := NewInProcessBroker()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := b.Subscribe(ctx, topicA)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed after context cancel")
	}

	if err := b.Publish(context.Background(), topicA, []byte("x")); err != nil {
		t.Fatalf("Publish after unsubscribe: %v", err)
	}
}

// TestInProcessBroker_SlowSubscriberDoesNotBlockOthers fills one subscriber's
// buffer and verifies publishing continues without blocking the publisher.
func TestInProcessBroker_SlowSubscriberDoesNotBlockOthers(t *testing.T) {
	t.Parallel()

	b := NewInProcessBroker()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slow, err := b.Subscribe(ctx, topicA)
	if err != nil {
		t.Fatalf("Subscribe slow: %v", err)
	}
	fast, err := b.Subscribe(ctx, topicA)
	if err != nil {
		t.Fatalf("Subscribe fast: %v", err)
	}

	const total = defaultSubscriberBuffer + 8
	publisherDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(publisherDone)
		for i := 0; i < total; i++ {
			_ = b.Publish(ctx, topicA, []byte("x"))
		}
	}()

	select {
	case <-publisherDone:
	case <-time.After(2 * time.Second):
		t.Fatal("publisher blocked behind slow subscriber")
	}
	wg.Wait()

	got := 0
drain:
	for {
		select {
		case <-fast:
			got++
		default:
			break drain
		}
	}
	if got == 0 {
		t.Fatal("fast subscriber received nothing")
	}

	for {
		select {
		case <-slow:
		default:
			return
		}
	}
}

// TestInProcessBroker_CloseClosesSubscribers ensures Close closes every
// outstanding subscriber channel.
func TestInProcessBroker_CloseClosesSubscribers(t *testing.T) {
	t.Parallel()

	b := NewInProcessBroker()

	ctx := context.Background()
	a, err := b.Subscribe(ctx, topicA)
	if err != nil {
		t.Fatalf("Subscribe a: %v", err)
	}
	c, err := b.Subscribe(ctx, topicB)
	if err != nil {
		t.Fatalf("Subscribe c: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for name, ch := range map[string]<-chan []byte{"a": a, "b": c} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("%s channel still open after Close", name)
			}
		case <-time.After(time.Second):
			t.Errorf("%s channel was not closed after Close", name)
		}
	}

	if err := b.Publish(context.Background(), topicA, []byte("x")); err != nil {
		t.Errorf("Publish after Close returned %v, want nil", err)
	}
}

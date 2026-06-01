package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// These tests cover the Redis-free surface: config resolution, option mapping,
// and task construction. Behaviour requiring a live Redis (enqueue/process/
// inspect) is exercised by the generated project's genvalidate run, not here,
// so the runtime module's tests stay hermetic (no Redis on PATH).

func TestRedisConfig_URLTakesPrecedence(t *testing.T) {
	cfg := RedisConfig{URL: "redis://:secret@example:6390/3", Addr: "ignored:1"}
	opt, err := cfg.redisOpt()
	if err != nil {
		t.Fatalf("redisOpt: %v", err)
	}
	got, ok := opt.(asynq.RedisClientOpt)
	if !ok {
		t.Fatalf("expected RedisClientOpt, got %T", opt)
	}
	if got.Addr != "example:6390" || got.Password != "secret" || got.DB != 3 {
		t.Errorf("URL not parsed: %+v", got)
	}
}

func TestRedisConfig_DefaultAddr(t *testing.T) {
	opt, err := RedisConfig{}.redisOpt()
	if err != nil {
		t.Fatalf("redisOpt: %v", err)
	}
	got := opt.(asynq.RedisClientOpt)
	if got.Addr != "127.0.0.1:6379" {
		t.Errorf("default addr = %q, want 127.0.0.1:6379", got.Addr)
	}
}

func TestRedisConfig_InvalidURL(t *testing.T) {
	if _, err := (RedisConfig{URL: "http://nope"}).redisOpt(); err == nil {
		t.Error("expected error for non-redis URL scheme")
	}
}

func TestNewTask_RoundTrips(t *testing.T) {
	task := NewTask("email:send", []byte(`{"to":"a@b.c"}`))
	if task.Type() != "email:send" {
		t.Errorf("Type = %q", task.Type())
	}
	if string(task.Payload()) != `{"to":"a@b.c"}` {
		t.Errorf("Payload = %q", task.Payload())
	}
}

func TestOptions_MapToAsynq(t *testing.T) {
	// Each Option must produce a non-nil asynq.Option; a nil here means a
	// wrapper silently dropped a setting.
	opts := []Option{
		Queue("critical"), MaxRetry(5), Delay(time.Minute), ProcessAt(time.Unix(0, 0)),
		Timeout(time.Second), Deadline(time.Unix(0, 0)), Unique(time.Hour),
		TaskID("abc"), Retention(24 * time.Hour),
	}
	mapped := asynqOpts(opts)
	if len(mapped) != len(opts) {
		t.Fatalf("mapped %d options, want %d", len(mapped), len(opts))
	}
	for i, o := range mapped {
		if o == nil {
			t.Errorf("option %d mapped to nil", i)
		}
	}
}

func TestNewServer_DefaultsAreUsable(t *testing.T) {
	// Zero-value Config must construct without a live Redis (connection is
	// lazy) and accept handler registration.
	srv, err := NewServer(Config{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	srv.Handle("noop", func(ctx context.Context, task *Task) error { return nil })
}

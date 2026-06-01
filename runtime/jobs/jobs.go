// Package jobs provides a small, opinionated wrapper around Asynq
// (github.com/hibiken/asynq), a Redis-backed background task queue. It is the
// imported, versioned half of ix's jobs block (see ../../docs/DESIGN.md §2):
// generated projects construct a Client to enqueue tasks, a Server to process
// them, and an Inspector to monitor/manage them, while the block vendors only
// the project-side wiring.
//
// Asynq guarantees at-least-once execution, so handlers MUST be idempotent.
// Because Redis is a separate datastore from the project's Postgres database,
// callers that enqueue a task alongside a database write should enqueue only
// AFTER the surrounding transaction commits — otherwise a worker can pick the
// task up before the row it depends on is visible. This is the two-store
// visibility race; ix's River alternative avoids it via transactional enqueue,
// but Asynq cannot. The vendored wire.go documents the post-commit pattern.
package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// RedisConfig identifies the Redis instance backing the queue. A non-empty URL
// (redis://, rediss://, or redis-socket://) takes precedence; otherwise Addr is
// used with optional Password and DB. Construct it from the project's env in
// wire.go.
type RedisConfig struct {
	// URL is a full Redis connection URL (e.g. redis://:pass@host:6379/0).
	// When set, Addr/Password/DB are ignored.
	URL string
	// Addr is host:port, used when URL is empty.
	Addr string
	// Password is the optional AUTH password, used when URL is empty.
	Password string
	// DB is the Redis logical database, used when URL is empty.
	DB int
}

// redisOpt resolves the config to an asynq.RedisConnOpt, validating the URL
// form when one is supplied.
func (c RedisConfig) redisOpt() (asynq.RedisConnOpt, error) {
	if c.URL != "" {
		opt, err := asynq.ParseRedisURI(c.URL)
		if err != nil {
			return nil, fmt.Errorf("jobs: parse redis url: %w", err)
		}
		return opt, nil
	}
	addr := c.Addr
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	return asynq.RedisClientOpt{Addr: addr, Password: c.Password, DB: c.DB}, nil
}

// Client enqueues tasks. It is safe for concurrent use and should be long-lived
// (one per process); Close it on shutdown.
type Client struct {
	inner *asynq.Client
}

// NewClient constructs a Client against the given Redis. The connection is
// lazy, so this does not fail on an unreachable Redis; the first Enqueue does.
func NewClient(cfg RedisConfig) (*Client, error) {
	opt, err := cfg.redisOpt()
	if err != nil {
		return nil, err
	}
	return &Client{inner: asynq.NewClient(opt)}, nil
}

// Enqueue schedules task for processing. Pass Options (Queue, MaxRetry, Delay,
// Deadline, Unique, …) to override per-task behaviour. It returns the enqueued
// task's info, whose ID and Queue identify it to the Inspector.
func (c *Client) Enqueue(ctx context.Context, task *Task, opts ...Option) (*TaskInfo, error) {
	info, err := c.inner.EnqueueContext(ctx, task.inner, asynqOpts(opts)...)
	if err != nil {
		return nil, fmt.Errorf("jobs: enqueue %q: %w", task.inner.Type(), err)
	}
	return &TaskInfo{inner: info}, nil
}

// Close releases the client's Redis connections.
func (c *Client) Close() error { return c.inner.Close() }

// Task is a unit of work: a stable type name plus an opaque payload. The type
// routes the task to a registered handler; the payload is whatever the handler
// decodes (JSON is the convention).
type Task struct {
	inner *asynq.Task
}

// NewTask builds a Task of the given type carrying payload. Marshal structured
// payloads to JSON before calling.
func NewTask(typ string, payload []byte) *Task {
	return &Task{inner: asynq.NewTask(typ, payload)}
}

// Type returns the task's routing type.
func (t *Task) Type() string { return t.inner.Type() }

// Payload returns the task's raw payload bytes.
func (t *Task) Payload() []byte { return t.inner.Payload() }

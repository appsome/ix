package jobs

import (
	"time"

	"github.com/hibiken/asynq"
)

// Option customizes how a task is enqueued. The set mirrors the Asynq options
// projects reach for most; it is re-exported here so call sites depend on this
// package rather than asynq directly.
type Option func() asynq.Option

func asynqOpts(opts []Option) []asynq.Option {
	out := make([]asynq.Option, 0, len(opts))
	for _, o := range opts {
		out = append(out, o())
	}
	return out
}

// Queue routes the task to the named queue (default "default"). Queues are
// weighted at the Server; see Config.Queues.
func Queue(name string) Option {
	return func() asynq.Option { return asynq.Queue(name) }
}

// MaxRetry sets how many times a failing task is retried before it is archived.
func MaxRetry(n int) Option {
	return func() asynq.Option { return asynq.MaxRetry(n) }
}

// Delay schedules the task to become available after d from now.
func Delay(d time.Duration) Option {
	return func() asynq.Option { return asynq.ProcessIn(d) }
}

// ProcessAt schedules the task to become available at a specific time.
func ProcessAt(t time.Time) Option {
	return func() asynq.Option { return asynq.ProcessAt(t) }
}

// Timeout bounds how long a single execution may run before it is treated as
// failed. Mutually exclusive with Deadline.
func Timeout(d time.Duration) Option {
	return func() asynq.Option { return asynq.Timeout(d) }
}

// Deadline fails the task if it has not completed by t. Mutually exclusive with
// Timeout.
func Deadline(t time.Time) Option {
	return func() asynq.Option { return asynq.Deadline(t) }
}

// Unique drops a duplicate enqueue of an identical (type+payload+queue) task
// while one is still pending within ttl. Use for idempotent dedup on the
// producer side; it is best-effort, not a hard guarantee.
func Unique(ttl time.Duration) Option {
	return func() asynq.Option { return asynq.Unique(ttl) }
}

// TaskID assigns an explicit ID so a duplicate enqueue with the same ID is
// rejected. Stronger than Unique when the caller owns a natural key.
func TaskID(id string) Option {
	return func() asynq.Option { return asynq.TaskID(id) }
}

// Retention keeps a completed task's result inspectable for d before it is
// deleted. Without it, successful tasks are removed immediately.
func Retention(d time.Duration) Option {
	return func() asynq.Option { return asynq.Retention(d) }
}

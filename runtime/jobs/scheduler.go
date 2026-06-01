package jobs

import (
	"context"
	"fmt"
	"sync"

	"github.com/hibiken/asynq"
)

// Scheduler enqueues recurring tasks on a cron schedule. It is separate from
// the Server: the Scheduler decides WHEN a task is enqueued; the Server's
// handlers decide what happens when one runs. Run exactly one Scheduler per
// cluster — running several double-enqueues every entry.
type Scheduler struct {
	inner *asynq.Scheduler
}

// NewScheduler constructs a Scheduler against the given Redis. Register entries
// with Register before running it.
func NewScheduler(cfg RedisConfig) (*Scheduler, error) {
	opt, err := cfg.redisOpt()
	if err != nil {
		return nil, err
	}
	return &Scheduler{inner: asynq.NewScheduler(opt, nil)}, nil
}

// Register schedules task to be enqueued on the given cron spec (standard
// 5-field cron, or the "@every 30s" / "@daily" macros). The returned entry ID
// identifies the schedule. opts apply to every enqueue of the entry.
func (s *Scheduler) Register(cronspec string, task *Task, opts ...Option) (string, error) {
	id, err := s.inner.Register(cronspec, task.inner, asynqOpts(opts)...)
	if err != nil {
		return "", fmt.Errorf("jobs: register schedule %q: %w", cronspec, err)
	}
	return id, nil
}

// Run starts the scheduler and blocks until ctx is cancelled. Start it in its
// own goroutine when co-located with other services.
func (s *Scheduler) Run(ctx context.Context) error {
	if err := s.inner.Start(); err != nil {
		return err
	}
	<-ctx.Done()
	s.inner.Shutdown()
	return nil
}

// Start launches the scheduler in the background and stops it when ctx is
// cancelled, decrementing wg on shutdown. Mirrors Server.Start.
func (s *Scheduler) Start(ctx context.Context, wg *sync.WaitGroup) error {
	if err := s.inner.Start(); err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		<-ctx.Done()
		s.inner.Shutdown()
		wg.Done()
	}()
	return nil
}

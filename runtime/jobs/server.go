package jobs

import (
	"context"
	"log"
	"sync"

	"github.com/hibiken/asynq"
)

// HandlerFunc processes a single task. Returning a non-nil error marks the task
// failed and schedules a retry (up to its MaxRetry); returning nil marks it
// complete. Handlers must be idempotent because delivery is at-least-once.
type HandlerFunc func(ctx context.Context, task *Task) error

// Config tunes the worker Server. The zero value is usable: Concurrency
// defaults to 10 and a single weighted "default" queue is used.
type Config struct {
	Redis RedisConfig
	// Concurrency caps simultaneously-executing tasks across all queues.
	Concurrency int
	// Queues maps queue name to relative weight. nil means {"default": 1}.
	// Higher-weighted queues are polled proportionally more often.
	Queues map[string]int
}

// Server processes enqueued tasks by dispatching them to registered handlers.
// Register handlers with Handle, then Run (blocking) or Start (background).
type Server struct {
	inner *asynq.Server
	mux   *asynq.ServeMux
}

// NewServer constructs a worker Server. Register handlers before running it.
func NewServer(cfg Config) (*Server, error) {
	opt, err := cfg.Redis.redisOpt()
	if err != nil {
		return nil, err
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}
	queues := cfg.Queues
	if len(queues) == 0 {
		queues = map[string]int{"default": 1}
	}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues:      queues,
	})
	return &Server{inner: srv, mux: asynq.NewServeMux()}, nil
}

// Handle registers fn as the handler for tasks of the given type. Call once per
// type before Run/Start; a type without a handler is retried then archived.
func (s *Server) Handle(taskType string, fn HandlerFunc) {
	s.mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
		return fn(ctx, &Task{inner: t})
	})
}

// Run starts processing and blocks until ctx is cancelled, then drains
// in-flight tasks and returns. Use this when the worker owns the process.
func (s *Server) Run(ctx context.Context) error {
	if err := s.inner.Start(s.mux); err != nil {
		return err
	}
	<-ctx.Done()
	s.inner.Shutdown()
	return nil
}

// Start launches processing in the background and stops it when ctx is
// cancelled, decrementing wg when the drain completes. Use this to co-locate a
// worker with the API server in one process; mirrors metric.Service.Start.
func (s *Server) Start(ctx context.Context, wg *sync.WaitGroup) error {
	if err := s.inner.Start(s.mux); err != nil {
		return err
	}
	log.Printf("Starting jobs worker")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		s.inner.Shutdown()
		wg.Done()
	}()
	return nil
}

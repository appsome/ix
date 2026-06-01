package jobs

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskInfo is the enqueue receipt returned by Client.Enqueue: enough to locate
// the task with the Inspector.
type TaskInfo struct {
	inner *asynq.TaskInfo
}

// ID is the task's unique identifier within its queue.
func (i *TaskInfo) ID() string { return i.inner.ID }

// Queue is the queue the task was enqueued to.
func (i *TaskInfo) Queue() string { return i.inner.Queue }

// Inspector provides read and management access to queues and their tasks. It
// backs the optional admin interface: queue stats, paginated task lists by
// state, and per-task actions (run-now, archive, delete). Construct one per
// process; Close it on shutdown.
type Inspector struct {
	inner *asynq.Inspector
}

// NewInspector constructs an Inspector against the given Redis.
func NewInspector(cfg RedisConfig) (*Inspector, error) {
	opt, err := cfg.redisOpt()
	if err != nil {
		return nil, err
	}
	return &Inspector{inner: asynq.NewInspector(opt)}, nil
}

// Close releases the inspector's Redis connections.
func (i *Inspector) Close() error { return i.inner.Close() }

// QueueStats is a point-in-time snapshot of one queue's task counts by state,
// suitable for a dashboard. Counts are independent (a task is in exactly one
// state).
type QueueStats struct {
	Queue     string `json:"queue"`
	Size      int    `json:"size"`
	Active    int    `json:"active"`
	Pending   int    `json:"pending"`
	Scheduled int    `json:"scheduled"`
	Retry     int    `json:"retry"`
	Archived  int    `json:"archived"`
	Completed int    `json:"completed"`
	Processed int    `json:"processed"`
	Failed    int    `json:"failed"`
	Paused    bool   `json:"paused"`
}

// Queues returns one QueueStats per known queue, for the dashboard overview.
func (i *Inspector) Queues() ([]QueueStats, error) {
	names, err := i.inner.Queues()
	if err != nil {
		return nil, fmt.Errorf("jobs: list queues: %w", err)
	}
	out := make([]QueueStats, 0, len(names))
	for _, q := range names {
		info, err := i.inner.GetQueueInfo(q)
		if err != nil {
			return nil, fmt.Errorf("jobs: queue info %q: %w", q, err)
		}
		out = append(out, QueueStats{
			Queue:     info.Queue,
			Size:      info.Size,
			Active:    info.Active,
			Pending:   info.Pending,
			Scheduled: info.Scheduled,
			Retry:     info.Retry,
			Archived:  info.Archived,
			Completed: info.Completed,
			Processed: info.Processed,
			Failed:    info.Failed,
			Paused:    info.Paused,
		})
	}
	return out, nil
}

// State names a task lifecycle state for listing. Use the exported constants.
type State string

const (
	StateActive    State = "active"
	StatePending   State = "pending"
	StateScheduled State = "scheduled"
	StateRetry     State = "retry"
	StateArchived  State = "archived"
	StateCompleted State = "completed"
)

// ListedTask is a task as shown in a management list: identity, payload, and
// the failure context an operator needs to decide whether to retry or delete.
type ListedTask struct {
	ID         string     `json:"id"`
	Queue      string     `json:"queue"`
	Type       string     `json:"type"`
	Payload    []byte     `json:"payload"`
	State      State      `json:"state"`
	MaxRetry   int        `json:"maxRetry"`
	Retried    int        `json:"retried"`
	LastErr    string     `json:"lastErr"`
	NextProcAt *time.Time `json:"nextProcessAt,omitempty"`
}

// ListTasks returns tasks in queue that are in the given state, paginated
// (1-based page, pageSize per page). Pending/Active states ignore ordering;
// Scheduled/Retry/Archived/Completed are ordered by their next-process time.
func (i *Inspector) ListTasks(queue string, state State, page, pageSize int) ([]ListedTask, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	opts := []asynq.ListOption{asynq.PageSize(pageSize), asynq.Page(page)}

	var (
		infos []*asynq.TaskInfo
		err   error
	)
	switch state {
	case StateActive:
		infos, err = i.inner.ListActiveTasks(queue, opts...)
	case StatePending:
		infos, err = i.inner.ListPendingTasks(queue, opts...)
	case StateScheduled:
		infos, err = i.inner.ListScheduledTasks(queue, opts...)
	case StateRetry:
		infos, err = i.inner.ListRetryTasks(queue, opts...)
	case StateArchived:
		infos, err = i.inner.ListArchivedTasks(queue, opts...)
	case StateCompleted:
		infos, err = i.inner.ListCompletedTasks(queue, opts...)
	default:
		return nil, fmt.Errorf("jobs: unknown task state %q", state)
	}
	if err != nil {
		return nil, fmt.Errorf("jobs: list %s tasks in %q: %w", state, queue, err)
	}

	out := make([]ListedTask, 0, len(infos))
	for _, ti := range infos {
		lt := ListedTask{
			ID:       ti.ID,
			Queue:    ti.Queue,
			Type:     ti.Type,
			Payload:  ti.Payload,
			State:    state,
			MaxRetry: ti.MaxRetry,
			Retried:  ti.Retried,
			LastErr:  ti.LastErr,
		}
		if !ti.NextProcessAt.IsZero() {
			t := ti.NextProcessAt
			lt.NextProcAt = &t
		}
		out = append(out, lt)
	}
	return out, nil
}

// RunTask promotes a scheduled, retry, or archived task to run immediately.
func (i *Inspector) RunTask(queue, id string) error {
	if err := i.inner.RunTask(queue, id); err != nil {
		return fmt.Errorf("jobs: run task %s/%s: %w", queue, id, err)
	}
	return nil
}

// ArchiveTask moves a task to the archive, stopping further retries while
// keeping it inspectable.
func (i *Inspector) ArchiveTask(queue, id string) error {
	if err := i.inner.ArchiveTask(queue, id); err != nil {
		return fmt.Errorf("jobs: archive task %s/%s: %w", queue, id, err)
	}
	return nil
}

// DeleteTask permanently removes a task from its queue.
func (i *Inspector) DeleteTask(queue, id string) error {
	if err := i.inner.DeleteTask(queue, id); err != nil {
		return fmt.Errorf("jobs: delete task %s/%s: %w", queue, id, err)
	}
	return nil
}

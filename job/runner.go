// Package job provides a lightweight task runner for managing long-running
// background goroutines with graceful shutdown support.
//
// The TaskRunner starts each registered Task in its own goroutine, propagates
// a cancellation context on shutdown, invokes cleanup callbacks, and waits
// for everything to finish within a configurable timeout.
//
// For periodic or cron-based scheduling, users can bring their own scheduler
// (e.g., github.com/go-co-op/gocron/v2) and register it as a Task.
package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Task represents a long-running background function managed by the TaskRunner.
//
// The Run function receives a context that is cancelled during application
// shutdown. Implementations MUST respect ctx.Done() and return promptly
// when the context is cancelled.
type Task struct {
	// Name is a human-readable identifier used in logs and error messages.
	Name string

	// Run executes the task. It blocks until the work is done or ctx is
	// cancelled. Return nil on clean shutdown; return an error to signal
	// a failure (which is logged but does not stop other tasks).
	Run func(ctx context.Context) error

	// Shutdown is an optional cleanup function called after Run returns
	// during graceful shutdown. It receives a fresh context with the
	// remaining shutdown timeout (not the cancelled task context), so
	// context-aware operations like closing connections still work.
	//
	// Per-task Shutdown callbacks run after all Run functions have returned,
	// in reverse registration order, before global OnShutdown callbacks.
	Shutdown func(ctx context.Context) error

	// RestartOnFailure controls whether the task is automatically restarted
	// after a panic or non-nil error return. When true, the task is restarted
	// with exponential backoff (1s, 2s, 4s, …, capped at maxRestartBackoff).
	// The backoff resets after a successful run lasting longer than the
	// current backoff interval.
	//
	// During shutdown (context cancelled), the task is NOT restarted
	// regardless of this setting.
	//
	// Default: false — the task exits permanently on failure.
	RestartOnFailure bool
}

const (
	// initialRestartBackoff is the starting delay before restarting a failed task.
	initialRestartBackoff = 1 * time.Second

	// maxRestartBackoff is the maximum delay between restart attempts.
	maxRestartBackoff = 1 * time.Minute
)

// shutdownCallback is a named cleanup function invoked during shutdown.
type shutdownCallback struct {
	name string
	fn   func(ctx context.Context) error
}

// TaskRunner manages background goroutines and shutdown callbacks.
//
// Usage:
//
//	r := job.NewTaskRunner(logger)
//	r.Add(job.Task{Name: "worker", Run: workerFunc})
//	r.OnShutdown("cleanup", cleanupFunc)
//	r.Start(ctx)
//	// ... later ...
//	r.Shutdown(30 * time.Second)
type TaskRunner struct {
	logger    *slog.Logger
	mu        sync.Mutex
	tasks     []Task
	callbacks []shutdownCallback
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	started   bool
}

// NewTaskRunner creates a TaskRunner that logs lifecycle events to the given logger.
func NewTaskRunner(logger *slog.Logger) *TaskRunner {
	return &TaskRunner{
		logger: logger,
	}
}

// Add registers a task to be started when Start is called.
// Add must be called before Start; adding tasks after Start is a no-op
// (the task will not be launched).
func (r *TaskRunner) Add(t Task) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks = append(r.tasks, t)
}

// OnShutdown registers a callback that is invoked during graceful shutdown.
// Callbacks run in LIFO order (last registered, first called) and receive
// a context bounded by the shutdown timeout.
func (r *TaskRunner) OnShutdown(name string, fn func(ctx context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.callbacks = append(r.callbacks, shutdownCallback{name: name, fn: fn})
}

// Start launches all registered tasks in separate goroutines.
// The provided context serves as the parent; cancelling it (or calling
// Shutdown) signals all tasks to stop.
// Start returns immediately. It is safe to call Start only once.
//
// When using App.Run(), Start is called automatically — you do not need
// to call it yourself. This method is public for users who use TaskRunner
// independently of App.
func (r *TaskRunner) Start(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return
	}
	r.started = true

	taskCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	for _, t := range r.tasks {
		r.wg.Add(1)

		go func(task Task) {
			defer r.wg.Done()
			r.runTask(taskCtx, task)
		}(t)
	}

	r.logger.Info("task runner started", "task.count", len(r.tasks))
}

// runTask executes a single task, handling panic recovery and optional
// restart-on-failure with exponential backoff.
func (r *TaskRunner) runTask(ctx context.Context, task Task) {
	backoff := initialRestartBackoff

	for {
		failed := r.runOnce(ctx, task)

		// If the context is done (shutdown), never restart.
		if ctx.Err() != nil {
			return
		}

		// If the task completed successfully or restart is not enabled, exit.
		if !failed || !task.RestartOnFailure {
			return
		}

		r.logger.Info("restarting task", "task", task.Name, "backoff", backoff)

		select {
		case <-time.After(backoff):
			backoff = min(backoff*2, maxRestartBackoff)
		case <-ctx.Done():
			r.logger.Info("task restart cancelled by shutdown", "task", task.Name)

			return
		}
	}
}

// runOnce executes task.Run once with panic recovery.
// Returns true if the task failed (error or panic), false if it completed cleanly.
func (r *TaskRunner) runOnce(ctx context.Context, task Task) (failed bool) {
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error("task panicked", "task", task.Name, "panic", p)
			failed = true
		}
	}()

	r.logger.Info("task started", "task", task.Name)

	if err := task.Run(ctx); err != nil {
		// Context cancellation is expected during shutdown — don't log as error.
		if ctx.Err() != nil {
			r.logger.Info("task stopped", "task", task.Name)
		} else {
			r.logger.Error("task failed", "task", task.Name, "error", err)
		}

		return ctx.Err() == nil // failed=true only if not a shutdown
	}

	r.logger.Info("task completed", "task", task.Name)

	return false
}

// Shutdown cancels the task context, waits for all Run functions to return,
// then invokes per-task Shutdown callbacks (LIFO), followed by global
// OnShutdown callbacks (LIFO). If tasks do not finish within the timeout,
// Shutdown returns an error.
func (r *TaskRunner) Shutdown(timeout time.Duration) error {
	r.mu.Lock()
	cancel := r.cancel
	tasks := make([]Task, len(r.tasks))
	copy(tasks, r.tasks)
	callbacks := make([]shutdownCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.Unlock()

	r.logger.Info("task runner shutting down")

	// Signal all tasks to stop.
	if cancel != nil {
		cancel()
	}

	// Create a deadline context for the entire shutdown sequence.
	ctx, cancelTimeout := context.WithTimeout(context.Background(), timeout)
	defer cancelTimeout()

	var errs []error

	// Wait for all Run functions to finish or timeout.
	done := make(chan struct{})

	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.logger.Info("all tasks stopped")
	case <-ctx.Done():
		r.logger.Error("task runner shutdown timed out waiting for tasks")
		errs = append(errs, fmt.Errorf("shutdown timed out after %v", timeout))

		return errors.Join(errs...)
	}

	// Run per-task Shutdown callbacks in LIFO order (last registered task first).
	for i := len(tasks) - 1; i >= 0; i-- {
		task := tasks[i]
		if task.Shutdown == nil {
			continue
		}

		r.logger.Info("running task shutdown", "task", task.Name)

		if err := task.Shutdown(ctx); err != nil {
			r.logger.Error("task shutdown failed", "task", task.Name, "error", err)
			errs = append(errs, fmt.Errorf("task %s shutdown: %w", task.Name, err))
		}
	}

	// Run global OnShutdown callbacks in LIFO order.
	for i := len(callbacks) - 1; i >= 0; i-- {
		cb := callbacks[i]
		r.logger.Info("running shutdown callback", "callback", cb.name)

		if err := cb.fn(ctx); err != nil {
			r.logger.Error("shutdown callback failed", "callback", cb.name, "error", err)
			errs = append(errs, fmt.Errorf("callback %s: %w", cb.name, err))
		}
	}

	return errors.Join(errs...)
}

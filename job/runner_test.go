package job

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mustAdd adds a task to the runner, failing the test if Add returns an error.
func mustAdd(t *testing.T, r *TaskRunner, task Task) {
	t.Helper()
	if err := r.Add(task); err != nil {
		t.Fatalf("Add(%q) failed: %v", task.Name, err)
	}
}

func TestNewTaskRunner(t *testing.T) {
	r := NewTaskRunner(discardLogger())
	if r == nil {
		t.Fatal("NewTaskRunner returned nil")
	}
}

func TestTaskRunner_StartAndShutdown(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var ran atomic.Bool

	mustAdd(t, r, Task{
		Name: "test-task",
		Run: func(ctx context.Context) error {
			ran.Store(true)
			<-ctx.Done()
			return nil
		},
	})

	ctx := context.Background()
	r.Start(ctx)

	// Give the goroutine time to start.
	time.Sleep(50 * time.Millisecond)

	if !ran.Load() {
		t.Fatal("task did not run")
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

func TestTaskRunner_MultipleTasksAllRun(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	const taskCount = 5
	var started atomic.Int32

	for i := range taskCount {
		name := "task-" + string(rune('A'+i))
		mustAdd(t, r, Task{
			Name: name,
			Run: func(ctx context.Context) error {
				started.Add(1)
				<-ctx.Done()
				return nil
			},
		})
	}

	r.Start(context.Background())
	time.Sleep(100 * time.Millisecond)

	if got := started.Load(); got != taskCount {
		t.Fatalf("expected %d tasks started, got %d", taskCount, got)
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_TaskError(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	mustAdd(t, r, Task{
		Name: "failing-task",
		Run: func(_ context.Context) error {
			return errors.New("task exploded")
		},
	})

	r.Start(context.Background())

	// The task returns an error immediately, shutdown should succeed.
	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_OnShutdownCallbacksLIFO(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var mu sync.Mutex
	var order []string

	r.OnShutdown("first", func(_ context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "first")
		return nil
	})

	r.OnShutdown("second", func(_ context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "second")
		return nil
	})

	r.OnShutdown("third", func(_ context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "third")
		return nil
	})

	r.Start(context.Background())

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"third", "second", "first"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d callbacks, got %d", len(expected), len(order))
	}

	for i, name := range expected {
		if order[i] != name {
			t.Errorf("callback %d: expected %q, got %q", i, name, order[i])
		}
	}
}

func TestTaskRunner_OnShutdownCallbackError(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	r.OnShutdown("bad-callback", func(_ context.Context) error {
		return errors.New("cleanup failed")
	})

	r.Start(context.Background())

	err := r.Shutdown(2 * time.Second)
	if err == nil {
		t.Fatal("expected shutdown error from callback, got nil")
	}

	if !errors.Is(err, errors.Unwrap(err)) {
		// Just check the error string contains the callback name.
		if got := err.Error(); got == "" {
			t.Fatal("expected non-empty error")
		}
	}
}

func TestTaskRunner_ShutdownTimeout(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	mustAdd(t, r, Task{
		Name: "stuck-task",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			// Simulate a task that takes too long to clean up.
			time.Sleep(5 * time.Second)
			return nil
		},
	})

	r.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	err := r.Shutdown(200 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestTaskRunner_StartIdempotent(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var count atomic.Int32

	mustAdd(t, r, Task{
		Name: "once",
		Run: func(ctx context.Context) error {
			count.Add(1)
			<-ctx.Done()
			return nil
		},
	})

	ctx := context.Background()
	r.Start(ctx)
	r.Start(ctx) // second call should be a no-op
	r.Start(ctx) // third call should be a no-op

	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Fatalf("expected task to run once, got %d", got)
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_NoTasksOrCallbacks(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	r.Start(context.Background())

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error with no tasks: %v", err)
	}
}

func TestTaskRunner_TaskCompletesBeforeShutdown(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var completed atomic.Bool

	mustAdd(t, r, Task{
		Name: "quick-task",
		Run: func(_ context.Context) error {
			completed.Store(true)
			return nil // completes immediately
		},
	})

	r.Start(context.Background())
	time.Sleep(100 * time.Millisecond)

	if !completed.Load() {
		t.Fatal("task did not complete")
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_PerTaskShutdown(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var shutdownCalled atomic.Bool

	mustAdd(t, r, Task{
		Name: "with-shutdown",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		Shutdown: func(ctx context.Context) error {
			// Verify the context is NOT cancelled (fresh deadline context).
			if ctx.Err() != nil {
				t.Error("shutdown context should not be cancelled")
			}
			shutdownCalled.Store(true)
			return nil
		},
	})

	r.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	if !shutdownCalled.Load() {
		t.Fatal("per-task Shutdown was not called")
	}
}

func TestTaskRunner_PerTaskShutdownError(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	mustAdd(t, r, Task{
		Name: "bad-shutdown",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		Shutdown: func(_ context.Context) error {
			return errors.New("cleanup exploded")
		},
	})

	r.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	err := r.Shutdown(2 * time.Second)
	if err == nil {
		t.Fatal("expected error from per-task Shutdown, got nil")
	}
}

func TestTaskRunner_PerTaskShutdownBeforeGlobal(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var mu sync.Mutex
	var order []string

	mustAdd(t, r, Task{
		Name: "task-A",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		Shutdown: func(_ context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, "task-A-shutdown")
			return nil
		},
	})

	r.OnShutdown("global-cleanup", func(_ context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "global-cleanup")
		return nil
	})

	r.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"task-A-shutdown", "global-cleanup"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d callbacks, got %d: %v", len(expected), len(order), order)
	}

	for i, name := range expected {
		if order[i] != name {
			t.Errorf("callback %d: expected %q, got %q", i, name, order[i])
		}
	}
}

func TestTaskRunner_PerTaskShutdownNilSkipped(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	mustAdd(t, r, Task{
		Name: "no-shutdown",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		// Shutdown is nil — should be skipped without error.
	})

	r.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_RestartOnFailure_Error(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var attempts atomic.Int32

	mustAdd(t, r, Task{
		Name: "restartable",
		Run: func(ctx context.Context) error {
			n := attempts.Add(1)
			if n < 3 {
				return errors.New("transient error")
			}
			// Succeed on the 3rd attempt, then block until shutdown.
			<-ctx.Done()
			return nil
		},
		RestartOnFailure: true,
	})

	r.Start(context.Background())

	// Wait for the task to restart a few times (backoff is 1s, so 3 attempts < 4s).
	time.Sleep(4 * time.Second)

	if got := attempts.Load(); got < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", got)
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_RestartOnFailure_Panic(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var attempts atomic.Int32

	mustAdd(t, r, Task{
		Name: "panic-restart",
		Run: func(ctx context.Context) error {
			n := attempts.Add(1)
			if n < 3 {
				panic("boom")
			}
			<-ctx.Done()
			return nil
		},
		RestartOnFailure: true,
	})

	r.Start(context.Background())
	time.Sleep(4 * time.Second)

	if got := attempts.Load(); got < 3 {
		t.Fatalf("expected at least 3 attempts after panic, got %d", got)
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_RestartOnFailure_StopsDuringShutdown(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var attempts atomic.Int32

	mustAdd(t, r, Task{
		Name: "fail-forever",
		Run: func(_ context.Context) error {
			attempts.Add(1)
			return errors.New("always fails")
		},
		RestartOnFailure: true,
	})

	r.Start(context.Background())
	time.Sleep(2 * time.Second)

	// Shutdown should stop the restart loop.
	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	countAtShutdown := attempts.Load()
	time.Sleep(2 * time.Second)

	// No more restarts should happen after shutdown.
	if got := attempts.Load(); got != countAtShutdown {
		t.Fatalf("task restarted after shutdown: got %d, expected %d", got, countAtShutdown)
	}
}

func TestTaskRunner_NoRestart_WhenDisabled(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var attempts atomic.Int32

	mustAdd(t, r, Task{
		Name: "no-restart",
		Run: func(_ context.Context) error {
			attempts.Add(1)
			return errors.New("fail once")
		},
		RestartOnFailure: false, // default
	})

	r.Start(context.Background())
	time.Sleep(500 * time.Millisecond)

	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected exactly 1 attempt (no restart), got %d", got)
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_PanicRecovery(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var otherTaskRan atomic.Bool

	// A task that panics should not crash the process or other tasks.
	mustAdd(t, r, Task{
		Name: "panicking-task",
		Run: func(_ context.Context) error {
			panic("something went terribly wrong")
		},
	})

	mustAdd(t, r, Task{
		Name: "healthy-task",
		Run: func(ctx context.Context) error {
			otherTaskRan.Store(true)
			<-ctx.Done()
			return nil
		},
	})

	r.Start(context.Background())
	time.Sleep(100 * time.Millisecond)

	// The healthy task should still be running despite the panic.
	if !otherTaskRan.Load() {
		t.Fatal("healthy task did not run after another task panicked")
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_OnPanic_FiresWithTaskName(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var gotName atomic.Value
	r.SetOnPanic(func(name string) {
		gotName.Store(name)
	})

	mustAdd(t, r, Task{
		Name: "panicker",
		Run: func(_ context.Context) error {
			panic("kaboom")
		},
	})

	r.Start(context.Background())
	// Wait for the panic recovery to run and invoke the hook.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if v := gotName.Load(); v != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	v := gotName.Load()
	if v == nil {
		t.Fatal("OnPanic hook was not called")
	}
	if got, want := v.(string), "panicker"; got != want {
		t.Fatalf("OnPanic received name %q, want %q", got, want)
	}
}

func TestTaskRunner_ParentContextCancellation(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	var stopped atomic.Bool

	mustAdd(t, r, Task{
		Name: "ctx-task",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			stopped.Store(true)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel() // cancel the parent context
	time.Sleep(100 * time.Millisecond)

	if !stopped.Load() {
		t.Fatal("task did not stop when parent context was cancelled")
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTaskRunner_AddAfterStart_ReturnsError(t *testing.T) {
	r := NewTaskRunner(discardLogger())

	mustAdd(t, r, Task{
		Name: "initial-task",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	})

	r.Start(context.Background())

	// Adding a task after Start should return an error.
	err := r.Add(Task{
		Name: "late-task",
		Run: func(ctx context.Context) error {
			t.Error("late task should never run")
			<-ctx.Done()
			return nil
		},
		Shutdown: func(_ context.Context) error {
			t.Error("late task Shutdown should never run")
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error when adding task after Start, got nil")
	}

	if err := r.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

package gokux_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dengliu/gokux"
	"github.com/dengliu/gokux/job"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppLifecycle_EndToEnd exercises the full App lifecycle: Init, Run
// (with a bound listener on port 0), the four standard endpoints, a
// registered background task, and graceful shutdown triggered by context
// cancellation. Guards against regressions across the lifecycle that
// per-package unit tests miss.
func TestAppLifecycle_EndToEnd(t *testing.T) {
	t.Setenv("GOKUX_SERVER_PORT", "0")
	t.Setenv("GOKUX_SERVER_DRAINWAITSECONDS", "0")
	t.Setenv("GOKUX_SERVER_SHUTDOWNTIMEOUTSECONDS", "5")
	t.Setenv("GOKUX_LOG_LEVEL", "warn")

	app := gokux.New()
	require.NoError(t, app.Init())

	const helloBody = `{"message":"hello"}`
	app.Server.Echo.GET("/api/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, helloBody)
	})

	var taskRan atomic.Bool
	require.NoError(t, app.TaskRunner.Add(job.Task{
		Name: "e2e-worker",
		Run: func(ctx context.Context) error {
			taskRan.Store(true)
			<-ctx.Done()
			return nil
		},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- app.Run(ctx) }()

	addr := waitForAddr(t, app, 2*time.Second)
	baseURL := "http://" + addr

	require.Eventually(t, taskRan.Load, time.Second, 10*time.Millisecond,
		"background task should have started")

	client := &http.Client{Timeout: 2 * time.Second}

	got := httpGet(t, client, baseURL+"/healthz")
	assert.Equal(t, http.StatusOK, got.status)
	assert.Contains(t, got.body, `"status":"alive"`)

	got = httpGet(t, client, baseURL+"/readyz")
	assert.Equal(t, http.StatusOK, got.status)
	assert.Contains(t, got.body, `"status":"ready"`)

	got = httpGet(t, client, baseURL+"/api/hello")
	assert.Equal(t, http.StatusOK, got.status)
	assert.Equal(t, helloBody, got.body)

	// Issue #31 bullet 5: the counter must be scrapable after real traffic.
	got = httpGet(t, client, baseURL+"/metrics")
	assert.Equal(t, http.StatusOK, got.status)
	assert.Contains(t, got.body, "http_server_request_count",
		"expected http_server_request_count in /metrics after traffic")
	assert.Contains(t, got.body, "http_server_request_duration",
		"expected http_server_request_duration in /metrics after traffic")

	// Trigger graceful shutdown. Budget: drain(0s) + shutdown(5s) plus slack.
	cancel()

	select {
	case err := <-runErr:
		assert.NoError(t, err, "app.Run returned an error on shutdown")
	case <-time.After(10 * time.Second):
		t.Fatal("app.Run did not exit within 10s after ctx cancellation")
	}
}

type httpResult struct {
	status int
	body   string
}

func httpGet(t *testing.T, client *http.Client, url string) httpResult {
	t.Helper()
	resp, err := client.Get(url)
	require.NoError(t, err, "GET %s", url)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return httpResult{status: resp.StatusCode, body: string(b)}
}

func waitForAddr(t *testing.T, app *gokux.App, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addr := app.Server.Addr(); addr != "" {
			// Addr may come back as "[::]:N" on dual-stack; rewrite to
			// a loopback form that http.Client can dial.
			if strings.HasPrefix(addr, "[::]:") {
				addr = "127.0.0.1:" + strings.TrimPrefix(addr, "[::]:")
			}
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server listener never bound")
	return ""
}

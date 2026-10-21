package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *logRecorder) log(stream, line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, stream+":"+line)
}

func (r *logRecorder) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

func TestRunCommandStreamsStdout(t *testing.T) {
	rec := &logRecorder{}
	if err := RunCommand(context.Background(), "echo lodestar-ok", "", rec.log); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(rec.joined(), "lodestar-ok") {
		t.Fatalf("expected command output to be streamed, got:\n%s", rec.joined())
	}
	if !strings.Contains(rec.joined(), "$ echo lodestar-ok") {
		t.Fatalf("expected the command itself to be logged, got:\n%s", rec.joined())
	}
}

func TestRunCommandReportsFailure(t *testing.T) {
	err := RunCommand(context.Background(), "exit 3", "", nil)
	if err == nil {
		t.Fatal("expected a non-zero exit to return an error")
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCommandEmptyIsNoop(t *testing.T) {
	if err := RunCommand(context.Background(), "   ", "", nil); err != nil {
		t.Fatalf("empty command should be a no-op, got %v", err)
	}
}

func TestRunCommandRespectsWorkingDir(t *testing.T) {
	dir := t.TempDir()
	rec := &logRecorder{}
	// `cd` prints the working directory on both cmd and sh.
	if err := RunCommand(context.Background(), "cd", dir, rec.log); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(rec.joined(), dir) && !strings.Contains(strings.ToLower(rec.joined()), strings.ToLower(dir)) {
		t.Fatalf("expected working dir %q in output, got:\n%s", dir, rec.joined())
	}
}

func TestWaitHealthySucceedsOn200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := WaitHealthy(context.Background(), server.URL, 2*time.Second, nil); err != nil {
		t.Fatalf("expected healthy, got %v", err)
	}
}

// 4xx is a live process that rejects the probe, not a broken deployment, so it
// must count as healthy — otherwise every service without a /healthz route
// would roll back.
func TestWaitHealthyAcceptsNon5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if err := WaitHealthy(context.Background(), server.URL, 2*time.Second, nil); err != nil {
		t.Fatalf("404 should be treated as alive, got %v", err)
	}
}

func TestWaitHealthyStopsOnCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := WaitHealthy(ctx, server.URL, 10*time.Second, nil)
	if err == nil {
		t.Fatal("expected a 503-only service to fail the health check")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation should interrupt the wait, took %s", elapsed)
	}
}

func TestWaitHealthyEmptyURLIsNoop(t *testing.T) {
	if err := WaitHealthy(context.Background(), "  ", time.Second, nil); err != nil {
		t.Fatalf("empty url should be a no-op, got %v", err)
	}
}

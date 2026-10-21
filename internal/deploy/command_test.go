package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// The command must run inside dir. Writing a file and looking for it there is
// portable: parsing shell output is not (`cd` prints the path under cmd.exe but
// prints nothing under /bin/sh).
func TestRunCommandRespectsWorkingDir(t *testing.T) {
	dir := t.TempDir()
	if err := RunCommand(context.Background(), "echo marker > marker.txt", dir, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil {
		t.Fatalf("expected marker.txt inside %s: %v", dir, err)
	}
	if !strings.Contains(string(body), "marker") {
		t.Fatalf("unexpected marker content: %q", body)
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

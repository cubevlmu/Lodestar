package deploy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Logger receives every line the pipeline produces. stream is one of
// info, stdout, stderr, error.
type Logger func(stream, line string)

// RunCommand executes a shell command inside dir and streams its output.
// On Windows the command runs through cmd /C, elsewhere through /bin/sh -c.
func RunCommand(ctx context.Context, command, dir string, logf Logger) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	if logf == nil {
		logf = func(string, string) {}
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	if dir != "" {
		cmd.Dir = dir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	logf("info", "$ "+command)
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan struct{}, 2)
	go scanStream(stdout, "stdout", logf, done)
	go scanStream(stderr, "stderr", logf, done)

	waitErr := cmd.Wait()
	<-done
	<-done
	if ctx.Err() != nil {
		return fmt.Errorf("command timed out or was cancelled: %w", ctx.Err())
	}
	if waitErr != nil {
		return fmt.Errorf("command failed: %w", waitErr)
	}
	return nil
}

func scanStream(r io.Reader, stream string, logf Logger, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		logf(stream, scanner.Text())
	}
}

// WaitHealthy polls an HTTP endpoint until it answers with a non-5xx status.
func WaitHealthy(ctx context.Context, url string, timeout time.Duration, logf Logger) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if logf == nil {
		logf = func(string, string) {}
	}

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0
	var lastErr error
	for {
		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				logf("info", fmt.Sprintf("health check passed (HTTP %d, attempt %d)", resp.StatusCode, attempt))
				return nil
			}
			lastErr = fmt.Errorf("health check returned HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("health check timed out")
			}
			return fmt.Errorf("health check failed after %d attempts: %w", attempt, lastErr)
		}
		logf("info", fmt.Sprintf("health check attempt %d failed, retrying: %v", attempt, lastErr))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

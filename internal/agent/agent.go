package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"lodestar/internal/buildinfo"
	"lodestar/internal/config"
	"lodestar/internal/deploy"
)

// jobPayload mirrors the server's agent job payload.
type jobPayload struct {
	ID             uint     `json:"id"`
	Mode           string   `json:"mode"`
	AppID          string   `json:"app_id"`
	Version        string   `json:"version"`
	VersionCode    int64    `json:"version_code"`
	PackageID      uint     `json:"package_id"`
	PackageName    string   `json:"package_name"`
	PackageSize    int64    `json:"package_size"`
	SHA256         string   `json:"sha256"`
	DownloadURL    string   `json:"download_url"`
	StopCommand    string   `json:"stop_command"`
	StartCommand   string   `json:"start_command"`
	RestartCommand string   `json:"restart_command"`
	HealthCheckURL string   `json:"health_check_url"`
	Preserve       []string `json:"preserve"`
	BackupKeep     int      `json:"backup_keep"`
	DeployDir      string   `json:"deploy_dir"`
	BackupDir      string   `json:"backup_dir"`
	Message        string   `json:"message"`
}

// appState is what the agent knows about one application on this host.
type appState struct {
	Version     string    `json:"version"`
	VersionCode int64     `json:"version_code"`
	JobID       uint      `json:"job_id"`
	DeployDir   string    `json:"deploy_dir"`
	DeployedAt  time.Time `json:"deployed_at"`
}

// agentState holds one entry per app_id: a single agent serves several
// applications, each with its own version and install directory.
type agentState struct {
	Apps map[string]appState `json:"apps"`
}

type Agent struct {
	cfg       config.AgentConfig
	client    *http.Client
	statePath string
	stateMu   sync.Mutex
	state     agentState
}

// Run starts the long-running agent loop. It only returns on context cancellation
// or a fatal configuration error.
func Run(ctx context.Context, cfg config.AgentConfig) error {
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return fmt.Errorf("agent.server_url is required")
	}
	if strings.TrimSpace(cfg.TargetToken) == "" {
		return fmt.Errorf("agent.target_token is required (copy it from the target page in the WebUI)")
	}
	if strings.TrimSpace(cfg.DeployRoot) == "" && strings.TrimSpace(cfg.DeployDir) == "" {
		return fmt.Errorf("agent.deploy_root is required (or set agent.deploy_dir for a single-app host)")
	}
	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	a := &Agent{
		cfg:       cfg,
		client:    &http.Client{Timeout: 0, Transport: transport},
		statePath: filepath.Join(cfg.WorkDir, "state.json"),
	}
	a.loadState()

	root := cfg.DeployRoot
	if root == "" {
		root = cfg.DeployDir
	}
	log.Printf("[agent] lodestar %s starting: server=%s target=%s root=%s", buildinfo.Version, cfg.ServerURL, cfg.Name, root)

	go a.heartbeatLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[agent] shutting down")
			return nil
		default:
		}

		job, err := a.nextJob(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("[agent] poll failed: %v", err)
			sleep(ctx, cfg.PollInterval)
			continue
		}
		if job == nil {
			continue
		}
		a.execute(ctx, job)
	}
}

// heartbeatLoop reports host state on a slow timer. Liveness is proven by the
// long poll, so this only exists to publish hostname/OS/versions — 15s was pure
// log noise.
func (a *Agent) heartbeatLoop(ctx context.Context) {
	interval := a.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	a.sendHeartbeat(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendHeartbeat(ctx)
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) {
	state := a.snapshot()
	hostname, _ := os.Hostname()
	apps := make([]map[string]any, 0, len(state.Apps))
	latestVersion, latestCode := "", int64(0)
	for appID, entry := range state.Apps {
		apps = append(apps, map[string]any{
			"app_id":       appID,
			"version":      entry.Version,
			"version_code": entry.VersionCode,
			"deploy_dir":   entry.DeployDir,
		})
		if entry.Version != "" {
			latestVersion, latestCode = entry.Version, entry.VersionCode
		}
	}
	body := map[string]any{
		"name":            a.cfg.Name,
		"hostname":        hostname,
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"agent_version":   buildinfo.Version,
		"current_version": latestVersion,
		"current_code":    latestCode,
		"apps":            apps,
	}
	var resp map[string]any
	if err := a.do(ctx, http.MethodPost, "/api/v1/agent/heartbeat", body, &resp); err != nil {
		log.Printf("[agent] heartbeat failed: %v", err)
	}
}

func (a *Agent) nextJob(ctx context.Context) (*jobPayload, error) {
	// The wait is the long-poll window, not the retry backoff: hold the
	// connection open so an idle agent makes ~1 request per minute instead of
	// reconnecting every few seconds.
	wait := longPollWait
	if a.cfg.PollInterval > wait {
		wait = a.cfg.PollInterval
	}
	path := fmt.Sprintf("/api/v1/agent/jobs/next?wait=%s", wait.String())
	reqCtx, cancel := context.WithTimeout(ctx, wait+30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, a.cfg.ServerURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Target-Token", a.cfg.TargetToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var payload struct {
			Job   *jobPayload `json:"job"`
			Error string      `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload.Error != "" {
			return nil, fmt.Errorf("%s", payload.Error)
		}
		return payload.Job, nil
	case http.StatusUnauthorized:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("target token rejected: %s", strings.TrimSpace(string(body)))
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (a *Agent) execute(ctx context.Context, job *jobPayload) {
	log.Printf("[agent] job #%d: %s %s (%s)", job.ID, job.AppID, job.Version, job.Mode)

	sink := &logSink{agent: a, jobID: job.ID}
	stopFlush := sink.start(ctx)
	defer func() {
		stopFlush()
		sink.flush(context.Background())
		// Publish the new version immediately: the regular heartbeat is slow
		// on purpose, and a deploy should not leave the UI stale for a minute.
		a.sendHeartbeat(context.Background())
	}()

	targetDir, err := a.resolveDeployDir(job)
	if err != nil {
		finish := map[string]any{
			"status":  "failed",
			"error":   err.Error(),
			"version": job.Version,
		}
		sink.log("error", err.Error())
		a.reportFinish(ctx, job.ID, finish, sink)
		return
	}
	sink.log("info", fmt.Sprintf("deploying %s %s to %s", job.AppID, job.Version, targetDir))

	spec := deploy.Spec{
		JobID:       job.ID,
		AppID:       job.AppID,
		Version:     job.Version,
		VersionCode: job.VersionCode,
		TargetDir:   targetDir,
		BackupDir:   firstNonEmpty(job.BackupDir, a.cfg.BackupDir),
		WorkDir:     a.cfg.WorkDir,
		// Commands and the whitelist come from the job (i.e. the per-app deploy
		// config) and nowhere else. The agent keeps no local fallback: an empty
		// field means "don't run anything", which is what the operator sees in
		// the WebUI.
		StopCommand:     a.command(job.StopCommand),
		StartCommand:    a.command(job.StartCommand),
		RestartCommand:  a.command(job.RestartCommand),
		HealthCheckURL:  a.command(job.HealthCheckURL),
		Preserve:        job.Preserve,
		BackupKeep:      pickBackupKeep(job.BackupKeep),
		MaxExtractBytes: 8 << 30,
		Timeout:         15 * time.Minute,
	}

	finish := map[string]any{
		"status":       "failed",
		"version":      job.Version,
		"version_code": job.VersionCode,
	}

	if job.Mode == "rollback" {
		result, err := deploy.Rollback(ctx, spec, sink.log)
		finish["backup_path"] = result.BackupPath
		finish["rolled_back"] = true
		finish["duration_ms"] = time.Since(result.StartedAt).Milliseconds()
		if err != nil {
			finish["error"] = err.Error()
			sink.log("error", "rollback failed: "+err.Error())
		} else {
			finish["status"] = "success"
			sink.log("info", "rollback completed")
			a.clearVersion(job)
		}
		a.reportFinish(ctx, job.ID, finish, sink)
		return
	}

	archivePath, err := a.downloadPackage(ctx, job)
	if err != nil {
		finish["error"] = err.Error()
		sink.log("error", "download failed: "+err.Error())
		a.reportFinish(ctx, job.ID, finish, sink)
		return
	}
	defer os.Remove(archivePath)

	spec.ArchivePath = archivePath
	result, runErr := deploy.Run(ctx, spec, sink.log)
	finish["backup_path"] = result.BackupPath
	finish["rolled_back"] = result.RolledBack
	finish["duration_ms"] = time.Since(result.StartedAt).Milliseconds()
	if runErr != nil {
		finish["error"] = runErr.Error()
	} else {
		finish["status"] = "success"
		a.saveVersion(job, targetDir)
	}
	a.reportFinish(ctx, job.ID, finish, sink)
}

func (a *Agent) reportFinish(ctx context.Context, jobID uint, body map[string]any, sink *logSink) {
	sink.flush(ctx)
	var resp map[string]any
	if err := a.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/agent/jobs/%d/finish", jobID), body, &resp); err != nil {
		log.Printf("[agent] report finish for job #%d failed: %v", jobID, err)
		return
	}
	status, _ := body["status"].(string)
	log.Printf("[agent] job #%d finished: %s", jobID, status)
}

func (a *Agent) downloadPackage(ctx context.Context, job *jobPayload) (string, error) {
	if job.DownloadURL == "" {
		return "", fmt.Errorf("job has no package")
	}
	url := job.DownloadURL
	if strings.HasPrefix(url, "/") {
		url = a.cfg.ServerURL + url
	}

	downloadDir := filepath.Join(a.cfg.WorkDir, "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return "", err
	}
	name := sanitize(job.PackageName)
	if name == "" {
		name = fmt.Sprintf("package-%d.bin", job.PackageID)
	}
	target := filepath.Join(downloadDir, fmt.Sprintf("%d-%s", job.ID, name))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Target-Token", a.cfg.TargetToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("download status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out, err := os.Create(target)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hasher), resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(target)
		return "", copyErr
	}
	if closeErr != nil {
		os.Remove(target)
		return "", closeErr
	}

	if job.PackageSize > 0 && written != job.PackageSize {
		os.Remove(target)
		return "", fmt.Errorf("size mismatch: expected %d, got %d", job.PackageSize, written)
	}
	if job.SHA256 != "" {
		actual := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(actual, job.SHA256) {
			os.Remove(target)
			return "", fmt.Errorf("sha256 mismatch: expected %s, got %s", job.SHA256, actual)
		}
	}
	log.Printf("[agent] downloaded %s (%d bytes)", name, written)
	return target, nil
}

// command returns the job's command unless the operator disabled remote
// commands on this host, in which case nothing runs.
func (a *Agent) command(value string) string {
	if a.cfg.AllowServerCommands != nil && !*a.cfg.AllowServerCommands {
		return ""
	}
	return strings.TrimSpace(value)
}

func pickBackupKeep(value int) int {
	if value > 0 {
		return value
	}
	return 5
}

func (a *Agent) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, a.cfg.ServerURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Target-Token", a.cfg.TargetToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s %s -> %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

func (a *Agent) loadState() {
	data, err := os.ReadFile(a.statePath)
	if err != nil {
		return
	}
	var state agentState
	if err := json.Unmarshal(data, &state); err == nil {
		if state.Apps == nil {
			state.Apps = map[string]appState{}
		}
		a.state = state
	}
}

func (a *Agent) snapshot() agentState {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.state
}

func (a *Agent) saveVersion(job *jobPayload, deployDir string) {
	a.stateMu.Lock()
	if a.state.Apps == nil {
		a.state.Apps = map[string]appState{}
	}
	a.state.Apps[job.AppID] = appState{
		Version:     job.Version,
		VersionCode: job.VersionCode,
		JobID:       job.ID,
		DeployDir:   deployDir,
		DeployedAt:  time.Now(),
	}
	state := a.state
	a.stateMu.Unlock()
	a.persistState(state)
}

func (a *Agent) clearVersion(job *jobPayload) {
	a.stateMu.Lock()
	if a.state.Apps != nil {
		delete(a.state.Apps, job.AppID)
	}
	state := a.state
	a.stateMu.Unlock()
	a.persistState(state)
}

func (a *Agent) persistState(state agentState) {
	if len(state.Apps) == 0 {
		os.Remove(a.statePath)
		return
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(a.statePath, data, 0o644); err != nil {
		log.Printf("[agent] save state: %v", err)
	}
}

// logSink batches log lines and ships them to the server while a job runs.
type logSink struct {
	agent *Agent
	jobID uint

	mu   sync.Mutex
	seq  int
	buf  []map[string]any
	once sync.Once
}

func (s *logSink) start(ctx context.Context) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.flush(ctx)
			}
		}
	}()
	return func() { s.once.Do(func() { close(stop) }) }
}

func (s *logSink) log(stream, line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	if len(line) > 8000 {
		line = line[:8000]
	}
	switch stream {
	case "info", "stdout", "stderr", "error":
	default:
		stream = "info"
	}
	s.mu.Lock()
	s.seq++
	entry := map[string]any{"seq": s.seq, "stream": stream, "line": line}
	s.buf = append(s.buf, entry)
	s.mu.Unlock()

	prefix := "[job]"
	switch stream {
	case "stderr", "error":
		log.Printf("%s %s", prefix, line)
	case "info":
		log.Printf("%s %s", prefix, line)
	}
}

func (s *logSink) flush(ctx context.Context) {
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()

	body := map[string]any{"lines": batch}
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	var resp map[string]any
	if err := s.agent.do(flushCtx, http.MethodPost, fmt.Sprintf("/api/v1/agent/jobs/%d/logs", s.jobID), body, &resp); err != nil {
		log.Printf("[agent] ship %d log lines for job #%d failed: %v", len(batch), s.jobID, err)
	}
}

func sanitize(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// resolveDeployDir decides where this job installs. Precedence: the job's own
// path, the agent's legacy single-app setting, then <root>/<app_id>.
//
// Any absolute path the operator configured is honoured; deploy_root only
// supplies the default when a job carries no path.
func (a *Agent) resolveDeployDir(job *jobPayload) (string, error) {
	root := strings.TrimSpace(a.cfg.DeployRoot)
	if root == "" {
		root = strings.TrimSpace(a.cfg.DeployDir)
	}

	raw := strings.TrimSpace(job.DeployDir)
	if raw == "" {
		raw = strings.TrimSpace(a.cfg.DeployDir)
	}
	if raw == "" {
		if root == "" {
			return "", fmt.Errorf("no deploy path for %s: set agent.deploy_root or pass deploy_dir", job.AppID)
		}
		raw = filepath.Join(root, sanitizeAppID(job.AppID))
	}

	if !filepath.IsAbs(raw) {
		if root == "" {
			return "", fmt.Errorf("relative deploy path %q needs agent.deploy_root", raw)
		}
		raw = filepath.Join(root, raw)
	}
	abs, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", fmt.Errorf("resolve deploy path %q: %w", raw, err)
	}
	// Refuse the filesystem root itself: mirroring a release into / or C:// would
	// delete everything else on the disk. Subdirectories anywhere are fine.
	if abs == string(filepath.Separator) || filepath.Dir(abs) == abs {
		return "", fmt.Errorf("refusing to deploy into filesystem root: %s", abs)
	}
	return abs, nil
}

// sanitizeAppID keeps an app id usable as a single path segment.
func sanitizeAppID(appID string) string {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, strings.TrimSpace(appID))
	clean = strings.Trim(clean, ".-")
	if clean == "" {
		return "app"
	}
	return clean
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// longPollWait is how long the agent keeps the job poll open. The server caps
// it, so this only needs to be long enough to keep an idle fleet quiet.
const longPollWait = 55 * time.Second

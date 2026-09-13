package warp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultTraceURL    = "https://www.cloudflare.com/cdn-cgi/trace"
	MaxSubprocessBytes = 64 * 1024
	PollInterval       = 500 * time.Millisecond
	ConnectTimeout     = 10 * time.Second
	DisconnectTimeout  = 5 * time.Second
)

var (
	ErrNotInstalled       = errors.New("warp-cli is not installed or not in PATH")
	ErrZeroTrustManaged   = errors.New("registration is managed by a Zero Trust organization")
	ErrDataPathUnverified = errors.New("warp-cli reported connected, but data-path verification failed (warp != on)")
	ErrConnectionTimeout  = errors.New("timed out waiting for WARP daemon to establish connection")
)

// RegistrationInfo holds parsed Cloudflare WARP registration metadata.
type RegistrationInfo struct {
	Registered     bool   `json:"registered"`
	AccountType    string `json:"account_type,omitempty"`
	Organization   string `json:"organization,omitempty"`
	IsOrganization bool   `json:"is_organization"`
}

// DaemonStatus represents the status reported by warp-cli status.
type DaemonStatus struct {
	Connected  bool   `json:"connected"`
	Connecting bool   `json:"connecting"`
	StatusText string `json:"status_text"`
}

// TraceInfo represents the parsed result of https://www.cloudflare.com/cdn-cgi/trace.
type TraceInfo struct {
	WarpActive    bool   `json:"warp_active"`
	IP            string `json:"ip"`
	Colo          string `json:"colo"`
	Location      string `json:"location"`
	GatewayActive bool   `json:"gateway_active"`
}

// Status aggregates all facets of WARP system state.
type Status struct {
	Installed      bool              `json:"installed"`
	BinaryPath     string            `json:"binary_path,omitempty"`
	Version        string            `json:"version,omitempty"`
	Registration   *RegistrationInfo `json:"registration,omitempty"`
	Daemon         *DaemonStatus     `json:"daemon,omitempty"`
	TunnelProtocol string            `json:"tunnel_protocol,omitempty"`
	Trace          *TraceInfo        `json:"trace,omitempty"`
	DiagnosticHint string            `json:"diagnostic_hint,omitempty"`
}

// Client interacts with the official Cloudflare warp-cli command-line tool.
type Client struct {
	BinaryPath string
	Version    string
	TraceURL   string
	HTTPClient *http.Client
	Logger     func(format string, args ...any)
}

// NewClient detects warp-cli in PATH and queries its version.
func NewClient() (*Client, error) {
	path, err := exec.LookPath("warp-cli")
	if err != nil {
		if runtime.GOOS == "darwin" {
			candidates := []string{
				"/usr/local/bin/warp-cli",
				"/opt/homebrew/bin/warp-cli",
				"/Applications/Cloudflare WARP.app/Contents/Resources/warp-cli",
			}
			for _, candidate := range candidates {
				if fi, statErr := os.Stat(candidate); statErr == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
					path = candidate
					err = nil
					break
				}
			}
		}
	}
	if err != nil {
		return nil, ErrNotInstalled
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	out, err := runBounded(cmd)
	version := strings.TrimSpace(string(out))
	if err != nil && version == "" {
		version = "unknown"
	}

	return &Client{
		BinaryPath: path,
		Version:    version,
		TraceURL:   DefaultTraceURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}, nil
}

func (c *Client) log(format string, args ...any) {
	if c.Logger != nil {
		c.Logger(format, args...)
	}
}

// GetRegistration parses warp-cli registration show.
func (c *Client) GetRegistration(ctx context.Context) (*RegistrationInfo, error) {
	cmd := exec.CommandContext(ctx, c.BinaryPath, "registration", "show")
	out, err := runBounded(cmd)
	text := string(out)

	info := &RegistrationInfo{}
	lower := strings.ToLower(text)

	if strings.Contains(lower, "not registered") ||
		strings.Contains(lower, "no registration") ||
		strings.Contains(lower, "registration missing") ||
		strings.Contains(lower, "registration not found") {
		info.Registered = false
		return info, nil
	}

	if err != nil {
		return nil, fmt.Errorf("warp-cli registration show failed: %w (%s)", err, strings.TrimSpace(text))
	}

	info.Registered = true
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		switch key {
		case "account type", "account":
			info.AccountType = val
			if strings.Contains(strings.ToLower(val), "team") ||
				strings.Contains(strings.ToLower(val), "organization") ||
				strings.Contains(strings.ToLower(val), "zero trust") {
				info.IsOrganization = true
			}
		case "organization", "team name":
			info.Organization = val
			if val != "" {
				info.IsOrganization = true
			}
		}
	}

	return info, nil
}

// Register registers a new consumer WARP client if not already registered.
func (c *Client) Register(ctx context.Context) error {
	reg, err := c.GetRegistration(ctx)
	if err != nil {
		return err
	}
	if reg.Registered {
		if reg.IsOrganization {
			c.log("Existing registration belongs to organization %q. Preserving configuration.", reg.Organization)
			return nil
		}
		c.log("Consumer WARP client already registered.")
		return nil
	}

	c.log("Registering consumer WARP client (warp-cli registration new)...")
	cmd := exec.CommandContext(ctx, c.BinaryPath, "registration", "new")
	out, err := runBounded(cmd)
	if err != nil {
		return fmt.Errorf("failed to register WARP client: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	// Verify registration succeeded
	verified, err := c.GetRegistration(ctx)
	if err != nil || !verified.Registered {
		return errors.New("registration command executed but client remains unregistered")
	}

	c.log("WARP client registered successfully.")
	return nil
}

// GetDaemonStatus checks warp-cli status.
func (c *Client) GetDaemonStatus(ctx context.Context) (*DaemonStatus, error) {
	cmd := exec.CommandContext(ctx, c.BinaryPath, "status")
	out, err := runBounded(cmd)
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return nil, fmt.Errorf("failed to query warp-cli status: %w", err)
	}

	lower := strings.ToLower(text)
	status := &DaemonStatus{StatusText: text}

	if strings.Contains(lower, "connected") &&
		!strings.Contains(lower, "disconnected") &&
		!strings.Contains(lower, "connecting") {
		status.Connected = true
	} else if strings.Contains(lower, "connecting") {
		status.Connecting = true
	}

	return status, nil
}

// GetProtocol inspects warp-cli settings for the configured tunnel protocol.
func (c *Client) GetProtocol(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, c.BinaryPath, "settings")
	out, err := runBounded(cmd)
	if err != nil {
		return "Unknown", nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "WARP tunnel protocol:") || strings.Contains(line, "Tunnel Protocol:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "Default", nil
}

// SetProtocol explicitly sets the tunnel protocol if requested.
// Valid values: "auto" (no-op), "masque", "wireguard".
func (c *Client) SetProtocol(ctx context.Context, proto string) error {
	p := strings.ToLower(strings.TrimSpace(proto))
	if p == "" || p == "auto" {
		return nil // Keep Cloudflare's default
	}

	var target string
	switch p {
	case "masque":
		target = "MASQUE"
	case "wireguard":
		target = "WireGuard"
	default:
		return fmt.Errorf("unsupported protocol %q: must be auto, masque, or wireguard", proto)
	}

	c.log("Setting WARP tunnel protocol to %s...", target)
	cmd := exec.CommandContext(ctx, c.BinaryPath, "tunnel", "protocol", "set", target)
	out, err := runBounded(cmd)
	if err != nil {
		return fmt.Errorf("failed to set tunnel protocol to %s: %v (%s)", target, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// VerifyTrace queries Cloudflare's official cdn-cgi/trace endpoint.
func (c *Client) VerifyTrace(ctx context.Context) (*TraceInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.TraceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "dorm-gateway/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trace request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trace endpoint returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read trace response: %w", err)
	}

	traceMap := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			traceMap[parts[0]] = parts[1]
		}
	}

	info := &TraceInfo{
		WarpActive:    traceMap["warp"] == "on",
		GatewayActive: traceMap["gateway"] == "on",
		IP:            traceMap["ip"],
		Colo:          traceMap["colo"],
		Location:      traceMap["loc"],
	}

	return info, nil
}

// Connect ensures registration, optionally sets protocol, connects WARP,
// polls daemon status, and strictly verifies data-path with Cloudflare trace.
func (c *Client) Connect(ctx context.Context, proto string) (*TraceInfo, error) {
	// 1. Ensure registration
	if err := c.Register(ctx); err != nil {
		return nil, fmt.Errorf("pre-connect registration failed: %w", err)
	}

	// 2. Set protocol if requested
	if err := c.SetProtocol(ctx, proto); err != nil {
		return nil, err
	}

	// 3. Initiate connect
	c.log("Initiating WARP connection (warp-cli connect)...")
	connectCmd := exec.CommandContext(ctx, c.BinaryPath, "connect")
	out, err := runBounded(connectCmd)
	if err != nil {
		return nil, fmt.Errorf("warp-cli connect failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	// 4. Poll warp-cli status
	c.log("Waiting for WARP daemon connection...")
	pollCtx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()

	connected := false
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for !connected {
		select {
		case <-pollCtx.Done():
			return nil, ErrConnectionTimeout
		case <-ticker.C:
			status, err := c.GetDaemonStatus(pollCtx)
			if err == nil && status.Connected {
				connected = true
				break
			}
		}
	}

	// 5. Data-path verification
	c.log("Daemon connected. Performing independent data-path verification (%s)...", c.TraceURL)
	trace, err := c.VerifyTrace(ctx)
	if err != nil {
		return nil, fmt.Errorf("data-path verification request failed: %w", err)
	}

	if !trace.WarpActive {
		return trace, ErrDataPathUnverified
	}

	c.log("WARP data-path verified active! (Public IP: %s, Colo: %s, Location: %s)", trace.IP, trace.Colo, trace.Location)
	return trace, nil
}

// Disconnect executes warp-cli disconnect and verifies disconnection.
func (c *Client) Disconnect(ctx context.Context) error {
	c.log("Disconnecting WARP (warp-cli disconnect)...")
	cmd := exec.CommandContext(ctx, c.BinaryPath, "disconnect")
	out, err := runBounded(cmd)
	if err != nil {
		return fmt.Errorf("warp-cli disconnect failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	pollCtx, cancel := context.WithTimeout(ctx, DisconnectTimeout)
	defer cancel()

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return nil // Best effort verification
		case <-ticker.C:
			status, err := c.GetDaemonStatus(pollCtx)
			if err == nil && !status.Connected && !status.Connecting {
				c.log("WARP disconnected successfully.")
				return nil
			}
		}
	}
}

// GetFullStatus compiles complete WARP diagnostics.
func (c *Client) GetFullStatus(ctx context.Context) *Status {
	st := &Status{
		Installed:  true,
		BinaryPath: c.BinaryPath,
		Version:    c.Version,
	}

	reg, err := c.GetRegistration(ctx)
	if err == nil {
		st.Registration = reg
	}

	daemon, err := c.GetDaemonStatus(ctx)
	if err == nil {
		st.Daemon = daemon
	}

	proto, err := c.GetProtocol(ctx)
	if err == nil {
		st.TunnelProtocol = proto
	}

	trace, err := c.VerifyTrace(ctx)
	if err == nil {
		st.Trace = trace
	}

	// Determine if warp-diag should be recommended
	if daemon != nil && daemon.Connected && (trace == nil || !trace.WarpActive) {
		st.DiagnosticHint = "Daemon reports connected but Cloudflare trace verification failed. Run 'warp-diag' to collect Cloudflare client diagnostic logs."
	}

	return st
}

// InstallGuidance returns concise, platform-specific guidance pointing to official Cloudflare sources.
func InstallGuidance() string {
	switch runtime.GOOS {
	case "darwin":
		return `Cloudflare WARP (warp-cli) is not installed or not in PATH.

Official macOS Installation:
- Via Homebrew (Recommended):
  brew install --cask cloudflare-warp

- Direct Download (.pkg):
  https://1.1.1.1 (Run the downloaded Cloudflare WARP.pkg installer)
  Verify by running 'warp-cli --version' in your terminal.`

	case "linux":
		distro := detectLinuxDistro()
		var guide strings.Builder
		guide.WriteString("Cloudflare WARP (cloudflare-warp) is not installed or not in PATH.\n\n")
		guide.WriteString("Official Cloudflare Package Repository: https://pkg.cloudflareclient.com/\n")
		guide.WriteString("Officially Supported Distributions: Ubuntu (22.04+), Debian (12+), RHEL/CentOS (9+), Fedora (43+)\n\n")

		switch distro {
		case "ubuntu", "debian":
			guide.WriteString("Debian/Ubuntu setup commands:\n")
			guide.WriteString("  curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | sudo gpg --yes --dearmor --output /usr/share/keyrings/cloudflare-warp-archive-keyring.gpg\n")
			guide.WriteString("  echo \"deb [signed-by=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg] https://pkg.cloudflareclient.com/ $(lsb_release -cs) main\" | sudo tee /etc/apt/sources.list.d/cloudflare-client.list\n")
			guide.WriteString("  sudo apt-get update && sudo apt-get install cloudflare-warp\n")
		case "rhel", "centos", "almalinux", "rocky":
			guide.WriteString("RHEL/CentOS setup commands (Requires EPEL):\n")
			guide.WriteString("  curl -fsSL https://pkg.cloudflareclient.com/cloudflare-warp-ascii.repo | sudo tee /etc/yum.repos.d/cloudflare-warp.repo\n")
			guide.WriteString("  sudo yum update && sudo yum install cloudflare-warp\n")
		case "fedora":
			guide.WriteString("Fedora setup commands:\n")
			guide.WriteString("  curl -fsSL https://pkg.cloudflareclient.com/cloudflare-warp-ascii.repo | sudo tee /etc/yum.repos.d/cloudflare-warp.repo\n")
			guide.WriteString("  sudo dnf update && sudo dnf install cloudflare-warp\n")
		default:
			guide.WriteString("Refer to https://pkg.cloudflareclient.com/ for instructions for your distribution.\n")
		}
		return guide.String()

	default:
		return "Please download the official Cloudflare WARP client from https://1.1.1.1"
	}
}

func detectLinuxDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.ToLower(strings.Trim(strings.TrimPrefix(line, "ID="), `"`))
		}
	}
	return "unknown"
}

func runBounded(cmd *exec.Cmd) ([]byte, error) {
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &limitWriter{w: &outBuf, limit: MaxSubprocessBytes}
	cmd.Stderr = &limitWriter{w: &errBuf, limit: MaxSubprocessBytes}

	err := cmd.Run()
	var combined []byte
	combined = append(combined, outBuf.Bytes()...)
	combined = append(combined, errBuf.Bytes()...)
	return combined, err
}

type limitWriter struct {
	w     io.Writer
	limit int64
	n     int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n >= l.limit {
		return len(p), nil
	}
	remain := l.limit - l.n
	if int64(len(p)) > remain {
		p = p[:remain]
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}

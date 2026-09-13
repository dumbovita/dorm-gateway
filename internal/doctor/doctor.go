package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/dumbovita/dorm-gateway/internal/auth"
	"github.com/dumbovita/dorm-gateway/internal/config"
	"github.com/dumbovita/dorm-gateway/internal/warp"
)

// Report contains full read-only diagnostic information.
type Report struct {
	OS              string
	Arch            string
	GoVersion       string
	ConfigPath      string
	ConfigExists    bool
	ConfigPerms     string
	ConfigPermsOK   bool
	UsernameMasked  string
	HasPassword     bool
	EnvUserSet      bool
	EnvPassSet      bool
	PortalResolves  bool
	PortalIPs       []string
	PortalError     string
	InternetActive  bool
	WarpInstalled   bool
	WarpBinary      string
	WarpVersion     string
	WarpRegistered  bool
	WarpAccountType string
	WarpOrg         string
	WarpDaemonState string
	WarpProtocol    string
	WarpTraceActive bool
	WarpTraceIP     string
	WarpTraceColo   string
	WarpTraceLoc    string
	Recommendations []string
}

// Options allows overriding targets for tests.
type Options struct {
	ConfigPath string
	PortalHost string
	CheckURL   string
	TraceURL   string
}

// RunDiagnostics executes all read-only diagnostic checks.
func RunDiagnostics(ctx context.Context, opts Options) *Report {
	r := &Report{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}

	portalHost := opts.PortalHost
	if portalHost == "" {
		portalHost = "wifi.gsb.gov.tr"
	}

	// 1. Config checks
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		p, err := config.DefaultConfigPath()
		if err == nil {
			cfgPath = p
		}
	}
	r.ConfigPath = cfgPath

	if info, err := os.Stat(cfgPath); err == nil {
		r.ConfigExists = true
		perm := info.Mode().Perm()
		r.ConfigPerms = fmt.Sprintf("%04o", perm)
		if runtime.GOOS != "windows" {
			r.ConfigPermsOK = (perm & 0077) == 0
			if !r.ConfigPermsOK {
				r.Recommendations = append(r.Recommendations,
					fmt.Sprintf("Config file permissions are overly permissive (%s). Run 'chmod 0600 %s'.", r.ConfigPerms, cfgPath))
			}
		} else {
			r.ConfigPermsOK = true
		}
	}

	cfg, _, _ := config.Load(cfgPath)
	if cfg != nil {
		r.UsernameMasked = config.MaskUsername(cfg.Username)
		r.HasPassword = len(cfg.Password) > 0
	}
	r.EnvUserSet = os.Getenv(config.EnvUsername) != "" || os.Getenv(config.EnvUsernameAlt1) != "" || os.Getenv(config.EnvUsernameAlt2) != ""
	r.EnvPassSet = os.Getenv(config.EnvPassword) != "" || os.Getenv(config.EnvPasswordAlt1) != "" || os.Getenv(config.EnvPasswordAlt2) != ""

	if !r.HasPassword {
		r.Recommendations = append(r.Recommendations,
			"No password configured. Run 'dorm-gateway config set' or export DORM_GATEWAY_PASSWORD.")
	}

	// 2. Portal DNS resolution & reachability
	ips, err := net.DefaultResolver.LookupHost(ctx, portalHost)
	if err == nil && len(ips) > 0 {
		r.PortalResolves = true
		r.PortalIPs = ips
	} else if err != nil {
		r.PortalError = err.Error()
	}

	// 3. Internet connectivity check
	authClient := auth.NewClient()
	if opts.CheckURL != "" {
		authClient.CheckURL = opts.CheckURL
	}
	if active, _ := authClient.CheckConnectivity(ctx); active {
		r.InternetActive = true
	} else {
		r.Recommendations = append(r.Recommendations,
			"Internet access is currently inactive. Run 'dorm-gateway auth' to authenticate to GSB WiFi.")
	}

	// 4. Cloudflare WARP checks
	warpClient, err := warp.NewClient()
	if err == nil {
		r.WarpInstalled = true
		r.WarpBinary = warpClient.BinaryPath
		r.WarpVersion = warpClient.Version
		if opts.TraceURL != "" {
			warpClient.TraceURL = opts.TraceURL
		}

		if reg, err := warpClient.GetRegistration(ctx); err == nil && reg != nil {
			r.WarpRegistered = reg.Registered
			r.WarpAccountType = reg.AccountType
			r.WarpOrg = reg.Organization
		}

		if daemon, err := warpClient.GetDaemonStatus(ctx); err == nil && daemon != nil {
			r.WarpDaemonState = daemon.StatusText
		}

		if proto, err := warpClient.GetProtocol(ctx); err == nil {
			r.WarpProtocol = proto
		}

		if trace, err := warpClient.VerifyTrace(ctx); err == nil && trace != nil {
			r.WarpTraceActive = trace.WarpActive
			r.WarpTraceIP = trace.IP
			r.WarpTraceColo = trace.Colo
			r.WarpTraceLoc = trace.Location
		}

		// Cloudflare health check recommendation
		if strings.Contains(strings.ToLower(r.WarpDaemonState), "connected") && !r.WarpTraceActive {
			r.Recommendations = append(r.Recommendations,
				"WARP daemon is connected but traffic is not verified as flowing through WARP. Run 'warp-diag' to collect Cloudflare client diagnostic logs.")
		}
	} else {
		r.Recommendations = append(r.Recommendations,
			"Cloudflare WARP (warp-cli) is not installed. Run 'dorm-gateway up' to install it automatically, or install via Homebrew ('brew install --cask cloudflare-warp') / https://1.1.1.1.")
	}

	return r
}

// Format formats the report into human-readable text.
func (r *Report) Format() string {
	var b strings.Builder

	b.WriteString("==================================================\n")
	b.WriteString("           DORM GATEWAY SYSTEM DIAGNOSTICS        \n")
	b.WriteString("==================================================\n\n")

	b.WriteString("[System & Runtime]\n")
	b.WriteString(fmt.Sprintf("  OS / Architecture : %s / %s\n", r.OS, r.Arch))
	b.WriteString(fmt.Sprintf("  Go Runtime        : %s\n\n", r.GoVersion))

	b.WriteString("[Credentials & Configuration]\n")
	b.WriteString(fmt.Sprintf("  Config Path       : %s\n", r.ConfigPath))
	b.WriteString(fmt.Sprintf("  Config Exists     : %t\n", r.ConfigExists))
	if r.ConfigExists {
		b.WriteString(fmt.Sprintf("  Permissions       : %s (Restricted 0600: %t)\n", r.ConfigPerms, r.ConfigPermsOK))
	}
	b.WriteString(fmt.Sprintf("  Username (TC)     : %s\n", r.UsernameMasked))
	b.WriteString(fmt.Sprintf("  Password Set      : %t\n", r.HasPassword))
	b.WriteString(fmt.Sprintf("  Env Overrides     : User: %t, Pass: %t\n\n", r.EnvUserSet, r.EnvPassSet))

	b.WriteString("[Network & Captive Portal]\n")
	b.WriteString(fmt.Sprintf("  Portal DNS Resolved: %t", r.PortalResolves))
	if r.PortalResolves {
		b.WriteString(fmt.Sprintf(" (%s)\n", strings.Join(r.PortalIPs, ", ")))
	} else {
		b.WriteString(fmt.Sprintf(" (Error: %s)\n", r.PortalError))
	}
	b.WriteString(fmt.Sprintf("  Internet Active   : %t\n\n", r.InternetActive))

	b.WriteString("[Cloudflare WARP]\n")
	b.WriteString(fmt.Sprintf("  warp-cli Installed: %t\n", r.WarpInstalled))
	if r.WarpInstalled {
		b.WriteString(fmt.Sprintf("  Binary Location   : %s\n", r.WarpBinary))
		b.WriteString(fmt.Sprintf("  Version           : %s\n", r.WarpVersion))
		b.WriteString(fmt.Sprintf("  Registered        : %t (Type: %s", r.WarpRegistered, r.WarpAccountType))
		if r.WarpOrg != "" {
			b.WriteString(fmt.Sprintf(", Org: %s", r.WarpOrg))
		}
		b.WriteString(")\n")
		b.WriteString(fmt.Sprintf("  Daemon Status     : %s\n", r.WarpDaemonState))
		b.WriteString(fmt.Sprintf("  Configured Protocol: %s\n", r.WarpProtocol))
		b.WriteString(fmt.Sprintf("  Trace Data-Path   : Active: %t", r.WarpTraceActive))
		if r.WarpTraceActive {
			b.WriteString(fmt.Sprintf(" (Public IP: %s, Colo: %s, Location: %s)", r.WarpTraceIP, r.WarpTraceColo, r.WarpTraceLoc))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString("[Recommendations & Verdict]\n")
	if len(r.Recommendations) == 0 {
		b.WriteString("  ✓ System is healthy and properly configured.\n")
	} else {
		for i, rec := range r.Recommendations {
			b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, rec))
		}
	}
	b.WriteString("==================================================\n")

	return b.String()
}

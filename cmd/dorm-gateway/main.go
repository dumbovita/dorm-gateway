package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dumbovita/dorm-gateway/internal/auth"
	"github.com/dumbovita/dorm-gateway/internal/config"
	"github.com/dumbovita/dorm-gateway/internal/doctor"
	"github.com/dumbovita/dorm-gateway/internal/warp"
)

const (
	Version = "2.0.0"

	ExitSuccess      = 0
	ExitGeneralError = 1
	ExitInvalidCreds = 2
	ExitNetworkError = 3
	ExitWarpError    = 4
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
		restoreTerminal()
		fmt.Fprintln(os.Stderr)
		os.Exit(130)
	}()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(ExitSuccess)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var exitCode int
	switch cmd {
	case "auth":
		exitCode = runAuth(ctx, args, false)
	case "up":
		// Convenience command: auth with --warp
		exitCode = runAuth(ctx, args, true)
	case "warp":
		exitCode = runWarp(ctx, args)
	case "status":
		exitCode = runStatus(ctx, args)
	case "config":
		exitCode = runConfig(args)
	case "doctor":
		exitCode = runDoctor(ctx, args)
	case "version", "-v", "-V", "-version", "--version":
		fmt.Printf("dorm-gateway version %s\n", Version)
		os.Exit(ExitSuccess)
	case "help", "-h", "--help":
		printUsage()
		os.Exit(ExitSuccess)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(ExitGeneralError)
	}

	os.Exit(exitCode)
}

func printUsage() {
	fmt.Println(`dorm-gateway - Fast macOS & Linux GSB WiFi Authentication CLI with Cloudflare WARP integration

Usage:
  dorm-gateway <command> [flags]

Commands:
  auth       Authenticate to GSB WiFi captive portal
  up         Authenticate to GSB WiFi and connect Cloudflare WARP (auth --warp)
  status     Check overall network connectivity and WARP status
  warp       Manage Cloudflare WARP (status, up, down)
  config     Manage credentials and settings (show, set, path)
  doctor     Run non-mutating system diagnostics and troubleshooting checks
  version    Show version information

Flags:
  Run 'dorm-gateway <command> --help' for details on a specific command.`)
}

func runAuth(ctx context.Context, args []string, forceWarp bool) int {
	fs := flag.NewFlagSet("auth", flag.ContinueOnError)
	warpFlag := fs.Bool("warp", forceWarp, "Connect Cloudflare WARP after successful authentication")
	protoFlag := fs.String("protocol", "auto", "Tunnel protocol to use with WARP (auto, masque, wireguard)")
	retriesFlag := fs.Int("retries", 5, "Maximum authentication retry attempts")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "Total deadline for authentication")
	configFlag := fs.String("config", "", "Custom path to config.json")
	verboseFlag := fs.Bool("verbose", false, "Enable verbose output")
	fs.BoolVar(verboseFlag, "v", false, "Enable verbose output (shorthand)")

	if err := fs.Parse(args); err != nil {
		return ExitGeneralError
	}

	cfg, _, err := config.Load(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		return ExitGeneralError
	}

	if cfg.Username == "" || cfg.Password == "" {
		if isTerminal(os.Stdin) {
			if promptErr := promptForCredentials(cfg); promptErr != nil {
				fmt.Fprintf(os.Stderr, "Credential input failed: %v\n", promptErr)
				return ExitGeneralError
			}
		} else {
			fmt.Fprintln(os.Stderr, "Error: Credentials not configured.")
			fmt.Fprintln(os.Stderr, "Please set them with 'dorm-gateway config set' or via environment variables (DORM_GATEWAY_USERNAME, DORM_GATEWAY_PASSWORD).")
			return ExitInvalidCreds
		}
	}

	authCtx, cancel := context.WithTimeout(ctx, *timeoutFlag)
	defer cancel()

	client := auth.NewClient()
	client.MaxAttempts = *retriesFlag
	if *verboseFlag {
		client.Logger = func(format string, a ...any) {
			fmt.Fprintf(os.Stderr, "[auth] "+format+"\n", a...)
		}
	}

	fmt.Printf("Authenticating user %s to GSB WiFi...\n", config.MaskUsername(cfg.Username))
	err = client.Authenticate(authCtx, cfg.Username, cfg.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			fmt.Fprintln(os.Stderr, "Authentication failed: Invalid TC Kimlik No or Password.")
			return ExitInvalidCreds
		}
		if errors.Is(err, auth.ErrPortalUnavailable) || errors.Is(err, auth.ErrNetworkUnavailable) {
			fmt.Fprintf(os.Stderr, "Network error: %v\n", err)
			return ExitNetworkError
		}
		fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
		return ExitGeneralError
	}

	fmt.Println("✓ GSB WiFi authentication successful. Internet access verified.")

	if *warpFlag {
		warpProto := *protoFlag
		if warpProto == "auto" && cfg.WarpProtocol != "" {
			warpProto = cfg.WarpProtocol
		}

		warpClient, err := warp.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nWarning: WARP requested, but warp-cli is not installed.\n%s\n", warp.InstallGuidance())
			return ExitWarpError
		}

		if *verboseFlag {
			warpClient.Logger = func(format string, a ...any) {
				fmt.Fprintf(os.Stderr, "[warp] "+format+"\n", a...)
			}
		}

		fmt.Printf("\nConnecting Cloudflare WARP (protocol: %s)...\n", warpProto)
		trace, err := warpClient.Connect(ctx, warpProto)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cloudflare WARP connection error: %v\n", err)
			return ExitWarpError
		}

		fmt.Printf("✓ Cloudflare WARP connected and data-path verified active (IP: %s, Colo: %s, Location: %s).\n",
			trace.IP, trace.Colo, trace.Location)
	}

	return ExitSuccess
}

func runWarp(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: dorm-gateway warp <status|up|down> [flags]")
		return ExitGeneralError
	}

	subcmd := args[0]
	subargs := args[1:]

	warpClient, err := warp.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n%s\n", err, warp.InstallGuidance())
		return ExitWarpError
	}

	switch subcmd {
	case "status":
		st := warpClient.GetFullStatus(ctx)
		fmt.Printf("warp-cli Binary: %s\n", st.BinaryPath)
		fmt.Printf("warp-cli Version: %s\n", st.Version)
		if st.Registration != nil {
			fmt.Printf("Registration: Registered: %t", st.Registration.Registered)
			if st.Registration.AccountType != "" {
				fmt.Printf(", Account: %s", st.Registration.AccountType)
			}
			if st.Registration.Organization != "" {
				fmt.Printf(", Organization: %s", st.Registration.Organization)
			}
			fmt.Println()
		}
		if st.Daemon != nil {
			fmt.Printf("Daemon Status: %s\n", st.Daemon.StatusText)
		}
		if st.TunnelProtocol != "" {
			fmt.Printf("Tunnel Protocol: %s\n", st.TunnelProtocol)
		}
		if st.Trace != nil {
			fmt.Printf("Data-Path Verification: WARP Active: %t", st.Trace.WarpActive)
			if st.Trace.WarpActive {
				fmt.Printf(" (Public IP: %s, Colo: %s, Location: %s)", st.Trace.IP, st.Trace.Colo, st.Trace.Location)
			}
			fmt.Println()
		}
		if st.DiagnosticHint != "" {
			fmt.Printf("\nDiagnostic Hint: %s\n", st.DiagnosticHint)
		}
		return ExitSuccess

	case "up", "connect":
		fs := flag.NewFlagSet("warp up", flag.ContinueOnError)
		protoFlag := fs.String("protocol", "auto", "Tunnel protocol (auto, masque, wireguard)")
		verboseFlag := fs.Bool("verbose", false, "Verbose output")
		fs.BoolVar(verboseFlag, "v", false, "Verbose output")
		if err := fs.Parse(subargs); err != nil {
			return ExitGeneralError
		}

		if *verboseFlag {
			warpClient.Logger = func(format string, a ...any) {
				fmt.Fprintf(os.Stderr, "[warp] "+format+"\n", a...)
			}
		}

		trace, err := warpClient.Connect(ctx, *protoFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARP connection failed: %v\n", err)
			return ExitWarpError
		}
		fmt.Printf("✓ Cloudflare WARP active and verified (IP: %s, Colo: %s, Location: %s)\n",
			trace.IP, trace.Colo, trace.Location)
		return ExitSuccess

	case "down", "disconnect":
		if err := warpClient.Disconnect(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "WARP disconnect failed: %v\n", err)
			return ExitWarpError
		}
		fmt.Println("✓ Cloudflare WARP disconnected.")
		return ExitSuccess

	default:
		fmt.Fprintf(os.Stderr, "Unknown warp command %q. Valid commands: status, up, down\n", subcmd)
		return ExitGeneralError
	}
}

func runStatus(ctx context.Context, args []string) int {
	authClient := auth.NewClient()
	online, _ := authClient.CheckConnectivity(ctx)

	fmt.Println("=== GSB WiFi Status ===")
	if online {
		fmt.Println("Internet Access: Online (Active)")
	} else {
		fmt.Println("Internet Access: Offline or Captive Portal Intercepted")
	}

	cfg, cfgPath, _ := config.Load("")
	if cfg != nil {
		fmt.Printf("Config File: %s\n", cfgPath)
		fmt.Printf("Configured User: %s (Password Set: %t)\n", config.MaskUsername(cfg.Username), len(cfg.Password) > 0)
	}

	fmt.Println("\n=== Cloudflare WARP Status ===")
	warpClient, err := warp.NewClient()
	if err != nil {
		fmt.Println("WARP: Not installed or not found in PATH")
	} else {
		st := warpClient.GetFullStatus(ctx)
		daemonState := "Unknown"
		if st.Daemon != nil {
			daemonState = st.Daemon.StatusText
		}
		fmt.Printf("WARP Daemon: %s\n", daemonState)
		if st.Trace != nil && st.Trace.WarpActive {
			fmt.Printf("WARP Tunnel: Active (Public IP: %s, Colo: %s, Location: %s)\n",
				st.Trace.IP, st.Trace.Colo, st.Trace.Location)
		} else {
			fmt.Println("WARP Tunnel: Inactive")
		}
	}

	return ExitSuccess
}

func runConfig(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: dorm-gateway config <show|set|path> [flags]")
		return ExitGeneralError
	}

	switch args[0] {
	case "show":
		cfg, path, err := config.Load("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			return ExitGeneralError
		}
		fmt.Printf("Config Path : %s\n", path)
		fmt.Printf("Username    : %s\n", config.MaskUsername(cfg.Username))
		fmt.Printf("Password Set: %t\n", len(cfg.Password) > 0)
		fmt.Printf("Protocol    : %s\n", cfg.WarpProtocol)
		return ExitSuccess

	case "path":
		path, err := config.DefaultConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return ExitGeneralError
		}
		fmt.Println(path)
		return ExitSuccess

	case "set":
		fs := flag.NewFlagSet("config set", flag.ContinueOnError)
		userFlag := fs.String("username", "", "TC Kimlik No (11 digits)")
		passFlag := fs.String("password", "", "Portal Password")
		protoFlag := fs.String("protocol", "", "Default WARP protocol (auto, masque, wireguard)")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitGeneralError
		}

		cfg, path, _ := config.Load("")
		if cfg == nil {
			cfg = &config.Config{WarpProtocol: "auto"}
		}

		if *userFlag != "" {
			cfg.Username = *userFlag
		}
		if *passFlag != "" {
			cfg.Password = *passFlag
		}
		if *protoFlag != "" {
			cfg.WarpProtocol = strings.ToLower(*protoFlag)
		}

		// Prompt interactively if missing and terminal attached
		if cfg.Username == "" && isTerminal(os.Stdin) {
			fmt.Print("Enter TC Kimlik No: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			cfg.Username = strings.TrimSpace(input)
		}
		if cfg.Password == "" && isTerminal(os.Stdin) {
			pass, err := readPassword("Enter Password: ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Password read error: %v\n", err)
				return ExitGeneralError
			}
			cfg.Password = pass
		}

		if err := cfg.Save(path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save configuration: %v\n", err)
			return ExitGeneralError
		}

		fmt.Printf("✓ Configuration saved to %s (permissions 0600)\n", path)
		return ExitSuccess

	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand %q. Valid commands: show, set, path\n", args[0])
		return ExitGeneralError
	}
}

func runDoctor(ctx context.Context, args []string) int {
	fmt.Println("Running GSB WiFi & Cloudflare WARP system diagnostics...")
	report := doctor.RunDiagnostics(ctx, doctor.Options{})
	fmt.Println(report.Format())
	return ExitSuccess
}

func promptForCredentials(cfg *config.Config) error {
	reader := bufio.NewReader(os.Stdin)
	if cfg.Username == "" {
		fmt.Print("Enter TC Kimlik No: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		cfg.Username = strings.TrimSpace(input)
	}

	if cfg.Password == "" {
		pass, err := readPassword("Enter Password: ")
		if err != nil {
			return err
		}
		cfg.Password = pass
	}

	// Ask to save
	fmt.Print("Save credentials to user config file for future use? [Y/n]: ")
	ans, _ := reader.ReadString('\n')
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans == "" || ans == "y" || ans == "yes" {
		if err := cfg.Save(""); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save credentials: %v\n", err)
		} else {
			fmt.Println("✓ Credentials saved securely.")
		}
	}

	return nil
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func restoreTerminal() {
	if isTerminal(os.Stdin) {
		cmd := exec.Command("stty", "echo")
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
	}
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if !isTerminal(os.Stdin) {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		return strings.TrimSpace(line), err
	}

	// Disable terminal echo via stty on Unix/macOS/Linux
	cmd := exec.Command("stty", "-echo")
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
	defer func() {
		restoreTerminal()
		fmt.Fprintln(os.Stderr)
	}()

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

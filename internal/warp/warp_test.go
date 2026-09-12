package warp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createFakeWarpCLI(t *testing.T) string {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "warp-cli")

	script := `#!/bin/sh
case "$1" in
  --version)
    echo "WARP 2026.7.1377.0"
    exit 0
    ;;
  registration)
    case "$2" in
      show)
        if [ "$FAKE_WARP_REG" = "org" ]; then
          echo "Account type: Team"
          echo "Organization: MyZeroTrustOrg"
          exit 0
        elif [ "$FAKE_WARP_REG" = "consumer" ]; then
          echo "Account type: Free"
          echo "Device ID: 1234-5678"
          exit 0
        elif [ "$FAKE_WARP_REG" = "daemon_down" ]; then
          echo "Error: Unable to connect to Cloudflare WARP daemon" >&2
          exit 1
        else
          echo "Error: Reason: Not registered"
          exit 1
        fi
        ;;
      new)
        export FAKE_WARP_REG="consumer"
        echo "Registration Succeeded"
        exit 0
        ;;
    esac
    ;;
  status)
    if [ "$FAKE_WARP_STATUS" = "disconnected" ]; then
      echo "Status update: Disconnected"
    elif [ "$FAKE_WARP_STATUS" = "connecting" ]; then
      echo "Status update: Connecting"
    else
      echo "Status update: Connected"
    fi
    exit 0
    ;;
  settings)
    echo "Merged configuration:"
    echo "WARP tunnel protocol: ${FAKE_WARP_PROTO:-MASQUE}"
    exit 0
    ;;
  tunnel)
    if [ "$2" = "protocol" ] && [ "$3" = "set" ]; then
      echo "Protocol updated to $4"
      exit 0
    fi
    ;;
  connect)
    exit 0
    ;;
  disconnect)
    exit 0
    ;;
  *)
    echo "Unknown command: $@"
    exit 1
    ;;
esac
`
	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create fake warp-cli: %v", err)
	}

	// Prepend tempDir to PATH for this test
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+origPath)

	return binPath
}

func TestWarpDetectionAndVersion(t *testing.T) {
	createFakeWarpCLI(t)

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client.Version != "WARP 2026.7.1377.0" {
		t.Errorf("expected version 'WARP 2026.7.1377.0', got %q", client.Version)
	}
}

func TestWarpRegistrationShow(t *testing.T) {
	createFakeWarpCLI(t)
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx := context.Background()

	// 1. Not registered
	t.Setenv("FAKE_WARP_REG", "none")
	reg, err := client.GetRegistration(ctx)
	if err != nil {
		t.Fatalf("GetRegistration failed: %v", err)
	}
	if reg.Registered {
		t.Errorf("expected unregistered")
	}

	// 2. Consumer registered
	t.Setenv("FAKE_WARP_REG", "consumer")
	reg, err = client.GetRegistration(ctx)
	if err != nil {
		t.Fatalf("GetRegistration failed: %v", err)
	}
	if !reg.Registered || reg.IsOrganization || reg.AccountType != "Free" {
		t.Errorf("expected registered consumer, got %+v", reg)
	}

	// 3. Organization registered
	t.Setenv("FAKE_WARP_REG", "org")
	reg, err = client.GetRegistration(ctx)
	if err != nil {
		t.Fatalf("GetRegistration failed: %v", err)
	}
	if !reg.Registered || !reg.IsOrganization || reg.Organization != "MyZeroTrustOrg" {
		t.Errorf("expected registered organization, got %+v", reg)
	}

	// 4. Daemon error distinguishes from unregistered
	t.Setenv("FAKE_WARP_REG", "daemon_down")
	_, err = client.GetRegistration(ctx)
	if err == nil {
		t.Errorf("expected error when daemon is down, got nil")
	}
}

func TestWarpProtocol(t *testing.T) {
	createFakeWarpCLI(t)
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx := context.Background()

	t.Setenv("FAKE_WARP_PROTO", "WireGuard")
	proto, err := client.GetProtocol(ctx)
	if err != nil {
		t.Fatalf("GetProtocol failed: %v", err)
	}
	if proto != "WireGuard" {
		t.Errorf("expected WireGuard, got %s", proto)
	}

	if err := client.SetProtocol(ctx, "wireguard"); err != nil {
		t.Errorf("SetProtocol wireguard failed: %v", err)
	}
	if err := client.SetProtocol(ctx, "masque"); err != nil {
		t.Errorf("SetProtocol masque failed: %v", err)
	}
	if err := client.SetProtocol(ctx, "auto"); err != nil {
		t.Errorf("SetProtocol auto failed: %v", err)
	}
	if err := client.SetProtocol(ctx, "invalid"); err == nil {
		t.Errorf("expected error for invalid protocol")
	}
}

func TestWarpConnectSuccessWithTraceVerification(t *testing.T) {
	createFakeWarpCLI(t)
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	traceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "fl=123\nip=104.28.1.1\ncolo=IST\nloc=TR\nwarp=on\ngateway=off\n")
	}))
	defer traceServer.Close()

	client.TraceURL = traceServer.URL
	t.Setenv("FAKE_WARP_REG", "consumer")
	t.Setenv("FAKE_WARP_STATUS", "connected")

	trace, err := client.Connect(context.Background(), "auto")
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !trace.WarpActive {
		t.Errorf("expected WarpActive to be true")
	}
	if trace.IP != "104.28.1.1" || trace.Colo != "IST" || trace.Location != "TR" {
		t.Errorf("unexpected trace info: %+v", trace)
	}
}

func TestWarpConnectFailsWhenTraceWarpOff(t *testing.T) {
	createFakeWarpCLI(t)
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Daemon reports connected, but Cloudflare trace reports warp=off
	traceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "fl=123\nip=185.1.1.1\ncolo=IST\nloc=TR\nwarp=off\ngateway=off\n")
	}))
	defer traceServer.Close()

	client.TraceURL = traceServer.URL
	t.Setenv("FAKE_WARP_REG", "consumer")
	t.Setenv("FAKE_WARP_STATUS", "connected")

	_, err = client.Connect(context.Background(), "auto")
	if !errors.Is(err, ErrDataPathUnverified) {
		t.Fatalf("expected ErrDataPathUnverified, got %v", err)
	}
}

func TestWarpDisconnect(t *testing.T) {
	createFakeWarpCLI(t)
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	t.Setenv("FAKE_WARP_STATUS", "disconnected")
	if err := client.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}
}

func TestInstallGuidance(t *testing.T) {
	guide := InstallGuidance()
	if guide == "" {
		t.Fatal("expected non-empty install guidance")
	}
}

func TestRunBoundedConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	bin := filepath.Join(tempDir, "concurrent_writer.sh")
	script := `#!/bin/sh
for i in $(seq 1 100); do
  echo "stdout line $i"
  echo "stderr line $i" >&2
done
`
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	cmd := exec.Command(bin)
	out, err := runBounded(cmd)
	if err != nil {
		t.Fatalf("runBounded failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
}

package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dumbovita/dorm-gateway/internal/config"
)

func TestRunDiagnostics(t *testing.T) {
	t.Setenv("DORM_GATEWAY_USERNAME", "")
	t.Setenv("GSBWIFI_USERNAME", "")
	t.Setenv("GSB_USERNAME", "")
	t.Setenv("DORM_GATEWAY_PASSWORD", "")
	t.Setenv("GSBWIFI_PASSWORD", "")
	t.Setenv("GSB_PASSWORD", "")

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	cfg := &config.Config{
		Username: "12345678901",
		Password: "testpassword",
	}
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config failed: %v", err)
	}

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkServer.Close()

	opts := Options{
		ConfigPath: configPath,
		PortalHost: "127.0.0.1",
		CheckURL:   checkServer.URL,
	}

	report := RunDiagnostics(context.Background(), opts)
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if !report.ConfigExists {
		t.Errorf("expected config to exist")
	}
	if !report.HasPassword {
		t.Errorf("expected password to be set")
	}
	if report.UsernameMasked != "123******01" {
		t.Errorf("expected masked username 123******01, got %s", report.UsernameMasked)
	}
	if !report.InternetActive {
		t.Errorf("expected internet to be active")
	}

	formatted := report.Format()
	if !strings.Contains(formatted, "DORM GATEWAY SYSTEM DIAGNOSTICS") {
		t.Errorf("expected formatted output to contain title")
	}
	if strings.Contains(formatted, "testpassword") {
		t.Errorf("diagnostics leaked the password!")
	}
}

func TestDoctorPermissivePermissionsWarning(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	_ = os.WriteFile(configPath, []byte(`{"username":"user","password":"pw"}`), 0666)

	opts := Options{
		ConfigPath: configPath,
		PortalHost: "127.0.0.1",
	}

	report := RunDiagnostics(context.Background(), opts)
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	// On unix-like systems, 0666 should trigger a recommendation
	if report.OS != "windows" {
		if report.ConfigPermsOK {
			t.Errorf("expected 0666 permissions to be flagged as not OK")
		}
	}
}

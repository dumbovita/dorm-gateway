package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	cfg := &Config{
		Username:     "12345678901",
		Password:     "supersecret123",
		WarpProtocol: "masque",
	}

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("cfg.Save failed: %v", err)
	}

	// Verify file permissions on POSIX
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("failed to stat config file: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Errorf("expected permissions 0600, got %o", perm)
		}
	}

	loaded, loadedPath, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loadedPath != configPath {
		t.Errorf("expected path %s, got %s", configPath, loadedPath)
	}
	if loaded.Username != cfg.Username {
		t.Errorf("expected username %s, got %s", cfg.Username, loaded.Username)
	}
	if loaded.Password != cfg.Password {
		t.Errorf("expected password %s, got %s", cfg.Password, loaded.Password)
	}
	if loaded.WarpProtocol != "masque" {
		t.Errorf("expected protocol masque, got %s", loaded.WarpProtocol)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fileCfg := &Config{
		Username:     "file_user",
		Password:     "file_pass",
		WarpProtocol: "wireguard",
	}
	if err := fileCfg.Save(configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	t.Setenv("DORM_GATEWAY_USERNAME", "env_user")
	t.Setenv("DORM_GATEWAY_PASSWORD", "env_pass")
	t.Setenv("DORM_GATEWAY_WARP_PROTOCOL", "masque")

	loaded, _, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Username != "env_user" {
		t.Errorf("expected env_user, got %s", loaded.Username)
	}
	if loaded.Password != "env_pass" {
		t.Errorf("expected env_pass, got %s", loaded.Password)
	}
	if loaded.WarpProtocol != "masque" {
		t.Errorf("expected masque, got %s", loaded.WarpProtocol)
	}
}

func TestMaskUsername(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"12345678901", "123******01"},
		{"", "(not set)"},
		{"   ", "(not set)"},
		{"abc", "***"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"myuser", "my**er"},
	}

	for _, tc := range tests {
		got := MaskUsername(tc.input)
		if got != tc.expected {
			t.Errorf("MaskUsername(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestStringDoesNotLeakPassword(t *testing.T) {
	secret := "topSecretPassword1234!"
	cfg := &Config{
		Username:     "12345678901",
		Password:     secret,
		WarpProtocol: "auto",
	}

	str := cfg.String()
	if strings.Contains(str, secret) {
		t.Fatalf("cfg.String() leaked the password! Got: %s", str)
	}
	if !strings.Contains(str, "123******01") {
		t.Errorf("cfg.String() should contain masked username, got: %s", str)
	}
}

func TestCustomPathErrorWhenMissing(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "non_existent_config.json")
	_, _, err := Load(missingPath)
	if err == nil {
		t.Fatalf("expected error when loading non-existent custom config path, got nil")
	}
}

func TestDefaultConfigMissingNotError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("DORM_GATEWAY_USERNAME", "")
	t.Setenv("GSBWIFI_USERNAME", "")
	t.Setenv("GSB_USERNAME", "")
	t.Setenv("DORM_GATEWAY_PASSWORD", "")
	t.Setenv("GSBWIFI_PASSWORD", "")
	t.Setenv("GSB_PASSWORD", "")
	t.Setenv("DORM_GATEWAY_WARP_PROTOCOL", "")
	t.Setenv("GSBWIFI_WARP_PROTOCOL", "")
	t.Setenv("GSB_WARP_PROTOCOL", "")

	cfg, _, err := Load("")
	if err != nil {
		t.Fatalf("expected no error when default config does not exist, got: %v", err)
	}
	if cfg.Username != "" {
		t.Errorf("expected empty username, got %s", cfg.Username)
	}
}

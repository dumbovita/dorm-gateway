package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIVersion(t *testing.T) {
	if Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", Version)
	}
}

func TestRunConfigSubcommands(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("HOME", tempDir)

	// Set credentials via config set flags
	setArgs := []string{"set", "-username", "12345678901", "-password", "mypassword", "-protocol", "masque"}
	exitCode := runConfig(setArgs)
	if exitCode != ExitSuccess {
		t.Fatalf("config set returned exit code %d", exitCode)
	}

	// Verify show
	exitCode = runConfig([]string{"show"})
	if exitCode != ExitSuccess {
		t.Fatalf("config show returned exit code %d", exitCode)
	}

	// Verify path
	exitCode = runConfig([]string{"path"})
	if exitCode != ExitSuccess {
		t.Fatalf("config path returned exit code %d", exitCode)
	}

	_ = configPath
}

func TestRunDoctor(t *testing.T) {
	ctx := context.Background()
	exitCode := runDoctor(ctx, nil)
	if exitCode != ExitSuccess {
		t.Errorf("doctor returned exit code %d", exitCode)
	}
}

func TestRunAuthMissingCredsNonInteractive(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "empty_config.json")
	_ = os.WriteFile(configPath, []byte(`{}`), 0600)

	ctx := context.Background()
	exitCode := runAuth(ctx, []string{"-config", configPath}, false)
	// Stdin in tests is not a terminal, so it should exit with ExitInvalidCreds or ExitGeneralError
	if exitCode != ExitInvalidCreds && exitCode != ExitGeneralError {
		t.Errorf("expected exit code %d or %d, got %d", ExitInvalidCreds, ExitGeneralError, exitCode)
	}
}

func TestIsTerminal(t *testing.T) {
	// A pipe is not a character device terminal
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()

	if isTerminal(r) {
		t.Errorf("pipe read end should not be detected as terminal")
	}
}

func TestPrintUsage(t *testing.T) {
	// Ensure printUsage executes without error
	var buf bytes.Buffer
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	_ = w.Close()
	os.Stdout = origStdout
	_, _ = buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "dorm-gateway <command>") {
		t.Errorf("usage output missing expected command syntax")
	}
}

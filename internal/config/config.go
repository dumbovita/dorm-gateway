package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	DirPerms  = 0700
	FilePerms = 0600

	EnvUsername     = "DORM_GATEWAY_USERNAME"
	EnvUsernameAlt1 = "GSBWIFI_USERNAME"
	EnvUsernameAlt2 = "GSB_USERNAME"

	EnvPassword     = "DORM_GATEWAY_PASSWORD"
	EnvPasswordAlt1 = "GSBWIFI_PASSWORD"
	EnvPasswordAlt2 = "GSB_PASSWORD"

	EnvProtocol     = "DORM_GATEWAY_WARP_PROTOCOL"
	EnvProtocolAlt1 = "GSBWIFI_WARP_PROTOCOL"
	EnvProtocolAlt2 = "GSB_WARP_PROTOCOL"

	DefaultAppName = "dorm-gateway"
	LegacyAppName  = "gsbwifi"
)

// Config holds the user credentials and operational settings.
type Config struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	WarpProtocol string `json:"warp_protocol,omitempty"`
}

// DefaultConfigDir returns the OS-specific user configuration directory.
func DefaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user config dir: %w", err)
	}
	return filepath.Join(base, DefaultAppName), nil
}

// DefaultConfigPath returns the standard configuration file path.
// If the legacy path exists and primary does not, it falls back to legacy path.
func DefaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user config dir: %w", err)
	}

	primary := filepath.Join(base, DefaultAppName, "config.json")
	if _, err := os.Stat(primary); err == nil {
		return primary, nil
	}

	legacy := filepath.Join(base, LegacyAppName, "config.json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy, nil
	}

	return primary, nil
}

// Load loads configuration with the following precedence:
// 1. Environment variables
// 2. Configuration file at custom path or default path
func Load(customPath string) (*Config, string, error) {
	cfg := &Config{
		WarpProtocol: "auto",
	}

	targetPath := customPath
	if customPath != "" {
		data, err := os.ReadFile(customPath)
		if err != nil {
			return nil, customPath, fmt.Errorf("failed to read config file %s: %w", customPath, err)
		}
		if runtime.GOOS != "windows" {
			if info, statErr := os.Stat(customPath); statErr == nil && info.Mode().Perm()&0077 != 0 {
				_ = os.Chmod(customPath, FilePerms)
			}
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, customPath, fmt.Errorf("malformed config file %s: %w", customPath, err)
		}
	} else {
		p, err := DefaultConfigPath()
		if err == nil {
			targetPath = p
			if data, err := os.ReadFile(targetPath); err == nil {
				if runtime.GOOS != "windows" {
					if info, statErr := os.Stat(targetPath); statErr == nil && info.Mode().Perm()&0077 != 0 {
						_ = os.Chmod(targetPath, FilePerms)
					}
				}
				if err := json.Unmarshal(data, cfg); err != nil {
					return nil, targetPath, fmt.Errorf("malformed config file %s: %w", targetPath, err)
				}
			} else if !os.IsNotExist(err) {
				return nil, targetPath, fmt.Errorf("failed to read config file %s: %w", targetPath, err)
			}
		}
	}

	// Environment variable overrides
	if u := getEnv(EnvUsername, EnvUsernameAlt1, EnvUsernameAlt2); u != "" {
		cfg.Username = u
	}
	if p := getEnv(EnvPassword, EnvPasswordAlt1, EnvPasswordAlt2); p != "" {
		cfg.Password = p
	}
	if pr := getEnv(EnvProtocol, EnvProtocolAlt1, EnvProtocolAlt2); pr != "" {
		cfg.WarpProtocol = strings.ToLower(pr)
	}

	if cfg.WarpProtocol == "" {
		cfg.WarpProtocol = "auto"
	}

	return cfg, targetPath, nil
}

// Save writes the configuration to disk with restrictive 0600 permissions.
func (c *Config) Save(targetPath string) error {
	if targetPath == "" {
		p, err := DefaultConfigPath()
		if err != nil {
			return err
		}
		targetPath = p
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, DirPerms); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	data = append(data, '\n')

	// Write with 0600 permissions atomically
	tmpFile := targetPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, FilePerms); err != nil {
		return fmt.Errorf("failed to write temporary config file: %w", err)
	}
	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	// Ensure permissions on POSIX
	if runtime.GOOS != "windows" {
		_ = os.Chmod(targetPath, FilePerms)
	}

	return nil
}

// MaskUsername conservatively masks username / TC ID for display.
// e.g. "12345678901" -> "123******01", "john" -> "jo**hn", "abc" -> "***"
func MaskUsername(u string) string {
	u = strings.TrimSpace(u)
	n := len(u)
	if n == 0 {
		return "(not set)"
	}
	if n == 11 {
		// Standard Turkish TC Kimlik format
		return u[:3] + "******" + u[9:]
	}
	if n > 4 {
		prefix := u[:2]
		suffix := u[n-2:]
		return prefix + strings.Repeat("*", n-4) + suffix
	}
	return strings.Repeat("*", n)
}

// String provides a safe string representation without ever leaking the password.
func (c *Config) String() string {
	hasPass := "no"
	if len(c.Password) > 0 {
		hasPass = "yes"
	}
	return fmt.Sprintf("Username: %s, Password Set: %s, WARP Protocol: %s",
		MaskUsername(c.Username), hasPass, c.WarpProtocol)
}

func getEnv(keys ...string) string {
	for _, k := range keys {
		if val := strings.TrimSpace(os.Getenv(k)); val != "" {
			return val
		}
	}
	return ""
}

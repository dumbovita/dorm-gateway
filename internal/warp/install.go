package warp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Install attempts to install Cloudflare WARP using the system's package manager or official package.
func Install(ctx context.Context) error {
	switch runtime.GOOS {
	case "darwin":
		return installDarwin(ctx)
	case "linux":
		return installLinux(ctx)
	default:
		return fmt.Errorf("automatic installation is not supported on %s; visit https://1.1.1.1", runtime.GOOS)
	}
}

func installDarwin(ctx context.Context) error {
	brewPath, err := exec.LookPath("brew")
	if err == nil {
		fmt.Println("Installing Cloudflare WARP via Homebrew (brew install --cask cloudflare-warp)...")
		cmd := exec.CommandContext(ctx, brewPath, "install", "--cask", "cloudflare-warp")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("brew installation failed: %w", err)
		}
		return postInstallSetup(ctx)
	}

	fmt.Println("Homebrew not found. Downloading official Cloudflare WARP package...")
	tempDir, err := os.MkdirTemp("", "warp-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	pkgPath := filepath.Join(tempDir, "Cloudflare_WARP.pkg")
	pkgURL := "https://downloads.cloudflareclient.com/v1/download/macos/ga"
	if err := downloadFile(ctx, pkgURL, pkgPath); err != nil {
		return fmt.Errorf("failed to download Cloudflare WARP package: %w", err)
	}

	fmt.Println("Installing package (administrator password may be requested)...")
	cmd := exec.CommandContext(ctx, "sudo", "installer", "-pkg", pkgPath, "-target", "/")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("macOS installer failed: %w", err)
	}

	return postInstallSetup(ctx)
}

func installLinux(ctx context.Context) error {
	distro := detectLinuxDistro()
	switch distro {
	case "ubuntu", "debian":
		fmt.Println("Installing Cloudflare WARP via APT (sudo password may be requested)...")
		script := `set -e
curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | sudo gpg --yes --dearmor --output /usr/share/keyrings/cloudflare-warp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg] https://pkg.cloudflareclient.com/ $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/cloudflare-client.list
sudo apt-get update
sudo apt-get install -y cloudflare-warp`
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("apt installation failed: %w", err)
		}

	case "fedora":
		fmt.Println("Installing Cloudflare WARP via DNF (sudo password may be required)...")
		script := `set -e
curl -fsSL https://pkg.cloudflareclient.com/cloudflare-warp-ascii.repo | sudo tee /etc/yum.repos.d/cloudflare-warp.repo
sudo dnf install -y cloudflare-warp`
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("dnf installation failed: %w", err)
		}

	case "rhel", "centos", "almalinux", "rocky":
		fmt.Println("Installing Cloudflare WARP via YUM (sudo password may be required)...")
		script := `set -e
curl -fsSL https://pkg.cloudflareclient.com/cloudflare-warp-ascii.repo | sudo tee /etc/yum.repos.d/cloudflare-warp.repo
sudo yum install -y cloudflare-warp`
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("yum installation failed: %w", err)
		}

	default:
		if _, err := os.Stat("/etc/arch-release"); err == nil {
			if yayPath, err := exec.LookPath("yay"); err == nil {
				fmt.Println("Installing Cloudflare WARP via yay...")
				cmd := exec.CommandContext(ctx, yayPath, "-S", "--noconfirm", "cloudflare-warp-bin")
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("yay installation failed: %w", err)
				}
				return postInstallSetup(ctx)
			}
		}
		return fmt.Errorf("unsupported Linux distribution %q for automatic installation.\n\n%s", distro, InstallGuidance())
	}

	return postInstallSetup(ctx)
}

func downloadFile(ctx context.Context, url string, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func postInstallSetup(ctx context.Context) error {
	fmt.Println("Finalizing Cloudflare WARP setup...")
	time.Sleep(2 * time.Second)

	client, err := NewClient()
	if err != nil {
		return fmt.Errorf("warp-cli not found after installation: %w", err)
	}

	reg, err := client.GetRegistration(ctx)
	if err != nil || reg == nil || !reg.Registered {
		fmt.Println("Registering client with Cloudflare WARP (warp-cli registration new)...")
		if regErr := client.Register(ctx); regErr != nil {
			fmt.Printf("Note: registration returned: %v (can be retried later with 'warp-cli registration new')\n", regErr)
		} else {
			fmt.Println("✓ Registered with Cloudflare WARP.")
		}
	}

	return nil
}

package adb

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Client encapsulates interactions with the Android Debug Bridge (ADB).
type Client struct {
	mu      sync.RWMutex
	adbPath string
}

// NewClient returns a new ADB client, searching for an existing binary or preparing the cache path.
func NewClient(customPath string) *Client {
	return &Client{
		adbPath: customPath,
	}
}

// GetADBPath returns the resolved ADB binary path.
func (c *Client) GetADBPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.adbPath
}

// SetADBPath manually overrides the ADB binary path.
func (c *Client) SetADBPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adbPath = path
}

// DefaultCacheDir returns the platform-specific directory where WaterStick stores downloaded tools.
func DefaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(appData, "waterstick", "bin"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Caches", "waterstick", "bin"), nil
	default:
		return filepath.Join(home, ".cache", "waterstick", "bin"), nil
	}
}

// FindADB checks the PATH or cache directory for a working ADB executable.
func (c *Client) FindADB() (string, error) {
	c.mu.RLock()
	if c.adbPath != "" {
		if _, err := os.Stat(c.adbPath); err == nil {
			c.mu.RUnlock()
			return c.adbPath, nil
		}
	}
	c.mu.RUnlock()

	// 1. Check system PATH
	if path, err := exec.LookPath("adb"); err == nil {
		c.SetADBPath(path)
		return path, nil
	}

	// 2. Check WaterStick cache directory
	cacheDir, err := DefaultCacheDir()
	if err == nil {
		binName := "adb"
		if runtime.GOOS == "windows" {
			binName = "adb.exe"
		}
		cachedAdb := filepath.Join(cacheDir, "platform-tools", binName)
		if _, err := os.Stat(cachedAdb); err == nil {
			c.SetADBPath(cachedAdb)
			return cachedAdb, nil
		}
	}

	return "", fmt.Errorf("adb executable not found on system")
}

// EnsureADB locates an existing ADB or automatically downloads Google platform-tools if missing.
func (c *Client) EnsureADB(ctx context.Context, progressFn func(percent int, msg string)) (string, error) {
	if path, err := c.FindADB(); err == nil {
		if progressFn != nil {
			progressFn(100, fmt.Sprintf("Using ADB binary at: %s", path))
		}
		return path, nil
	}

	if progressFn != nil {
		progressFn(5, "ADB not found. Starting automatic download of Google platform-tools...")
	}

	var downloadURL string
	switch runtime.GOOS {
	case "windows":
		downloadURL = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	case "darwin":
		downloadURL = "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip"
	case "linux":
		downloadURL = "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
	default:
		return "", fmt.Errorf("unsupported operating system for automated ADB download: %s", runtime.GOOS)
	}

	cacheDir, err := DefaultCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get cache directory: %w", err)
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	zipPath := filepath.Join(cacheDir, "platform-tools.zip")
	defer os.Remove(zipPath)

	// Download zip
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download platform-tools: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned HTTP %d while downloading platform-tools", resp.StatusCode)
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}

	var totalBytes int64 = resp.ContentLength
	var downloadedBytes int64
	buf := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			out.Close()
			return "", ctx.Err()
		default:
		}

		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				out.Close()
				return "", wErr
			}
			downloadedBytes += int64(n)
			if totalBytes > 0 && progressFn != nil {
				pct := int((downloadedBytes * 70) / totalBytes)
				progressFn(5+pct, fmt.Sprintf("Downloading platform-tools... (%d MB)", downloadedBytes/(1024*1024)))
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			out.Close()
			return "", rErr
		}
	}
	out.Close()

	if progressFn != nil {
		progressFn(80, "Extracting platform-tools archive...")
	}

	// Unzip
	if err := unzip(zipPath, cacheDir); err != nil {
		return "", fmt.Errorf("failed to extract platform-tools archive: %w", err)
	}

	binName := "adb"
	if runtime.GOOS == "windows" {
		binName = "adb.exe"
	}
	installedAdb := filepath.Join(cacheDir, "platform-tools", binName)

	if runtime.GOOS != "windows" {
		_ = os.Chmod(installedAdb, 0755)
	}

	if _, err := os.Stat(installedAdb); err != nil {
		return "", fmt.Errorf("extracted adb binary not found at %s: %w", installedAdb, err)
	}

	c.SetADBPath(installedAdb)
	if progressFn != nil {
		progressFn(100, "ADB ready!")
	}
	return installedAdb, nil
}

// Connect connects to a Fire TV over Wi-Fi via ADB.
func (c *Client) Connect(ctx context.Context, hostPort string) error {
	adbBin, err := c.FindADB()
	if err != nil {
		return err
	}

	if !strings.Contains(hostPort, ":") {
		hostPort = hostPort + ":5555"
	}

	cmd := exec.CommandContext(ctx, adbBin, "connect", hostPort)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		return fmt.Errorf("adb connect error: %s (%w)", strings.TrimSpace(output), err)
	}

	if strings.Contains(output, "connected to") || strings.Contains(output, "already connected") {
		return nil
	}

	if strings.Contains(output, "Connection refused") {
		return fmt.Errorf("connexion refusée sur %s (vérifiez que l'IP est bien celle de la Fire TV et non celle de votre box/routeur)", hostPort)
	}

	if strings.Contains(output, "failed to authenticate") {
		return fmt.Errorf("autorisation requise : validez le message sur l'écran de votre téléviseur avec la télécommande")
	}

	return fmt.Errorf("erreur de connexion: %s", strings.TrimSpace(output))
}

// Disconnect drops an active ADB connection.
func (c *Client) Disconnect(ctx context.Context, hostPort string) error {
	adbBin, err := c.FindADB()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, adbBin, "disconnect", hostPort)
	_ = cmd.Run()
	return nil
}

// ListDevices returns all connected ADB devices and their authorization state.
func (c *Client) ListDevices(ctx context.Context) ([]string, error) {
	adbBin, err := c.FindADB()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, adbBin, "devices")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var devices []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == "device" {
			devices = append(devices, parts[0])
		}
	}
	return devices, nil
}

// Shell executes a shell command on the specified target device.
func (c *Client) Shell(ctx context.Context, device string, shellCmd string) (string, error) {
	adbBin, err := c.FindADB()
	if err != nil {
		return "", err
	}

	args := []string{}
	if device != "" {
		args = append(args, "-s", device)
	}
	args = append(args, "shell", shellCmd)

	cmd := exec.CommandContext(ctx, adbBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("shell failed: %s (%w)", errMsg, err)
	}

	return stdout.String(), nil
}

// InstallAPK installs an APK package onto the target device.
func (c *Client) InstallAPK(ctx context.Context, device string, apkPath string) error {
	adbBin, err := c.FindADB()
	if err != nil {
		return err
	}

	args := []string{}
	if device != "" {
		args = append(args, "-s", device)
	}
	args = append(args, "install", "-r", "-d", apkPath)

	cmd := exec.CommandContext(ctx, adbBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Success") {
		return fmt.Errorf("install failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Helper to unzip an archive.
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Guard against zip slip
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

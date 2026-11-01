package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOwner = "MedCy1"
	defaultRepo  = "WaterStick"
	githubAPIFmt = "https://api.github.com/repos/%s/%s/releases/latest"
)

// ReleaseAsset represents an artifact attached to a GitHub release.
type ReleaseAsset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"browser_download_url"`
}

// ReleaseInfo holds metadata for a published GitHub release.
type ReleaseInfo struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

// UpdateChecker inspects and retrieves remote releases.
type UpdateChecker struct {
	Owner      string
	Repo       string
	HTTPClient *http.Client
}

// NewUpdateChecker creates a default checker for WaterStick.
func NewUpdateChecker() *UpdateChecker {
	return &UpdateChecker{
		Owner: defaultOwner,
		Repo:  defaultRepo,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// CheckLatest queries GitHub for the latest release and checks if it is newer than currentVersion.
func (u *UpdateChecker) CheckLatest(ctx context.Context, currentVersion string) (*ReleaseInfo, bool, error) {
	url := fmt.Sprintf(githubAPIFmt, u.Owner, u.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "WaterStick-Updater")
	if token := getGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var rel ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, false, fmt.Errorf("failed to decode release payload: %w", err)
	}

	isNewer := CompareVersions(rel.TagName, currentVersion) > 0
	return &rel, isNewer, nil
}

// CompareVersions compares two semver strings (e.g. "v1.0.1" and "v1.0.0").
// Returns 1 if v1 > v2, -1 if v1 < v2, and 0 if equal.
func CompareVersions(v1, v2 string) int {
	v1Clean := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	v2Clean := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	if v1Clean == "dev" || v1Clean == "" {
		return -1
	}
	if v2Clean == "dev" || v2Clean == "" {
		return 1
	}

	p1 := parseSemverParts(v1Clean)
	p2 := parseSemverParts(v2Clean)

	for i := 0; i < 3; i++ {
		if p1[i] > p2[i] {
			return 1
		}
		if p1[i] < p2[i] {
			return -1
		}
	}
	return 0
}

func parseSemverParts(v string) [3]int {
	var parts [3]int
	chunks := strings.Split(v, ".")
	for i := 0; i < len(chunks) && i < 3; i++ {
		numStr := chunks[i]
		// Strip any prerelease suffix like -rc1
		if idx := strings.IndexAny(numStr, "-+"); idx != -1 {
			numStr = numStr[:idx]
		}
		if n, err := strconv.Atoi(numStr); err == nil {
			parts[i] = n
		}
	}
	return parts
}

// TargetArchiveName determines the expected asset filename for the current OS and architecture.
func TargetArchiveName(tagName, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("waterstick_%s_%s_%s.%s", tagName, goos, goarch, ext)
}

// DownloadAsset streams a remote asset URL to memory.
func (u *UpdateChecker) DownloadAsset(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WaterStick-Updater")
	if token := getGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// VerifyChecksum checks that archiveBytes matches the sha256 checksum in checksums.txt content.
func VerifyChecksum(archiveBytes []byte, assetName, checksumsContent string) error {
	sum := sha256.Sum256(archiveBytes)
	actualHash := hex.EncodeToString(sum[:])

	lines := strings.Split(checksumsContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			hash := fields[0]
			name := fields[1]
			if strings.TrimPrefix(name, "*") == assetName {
				if strings.EqualFold(hash, actualHash) {
					return nil
				}
				return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, hash, actualHash)
			}
		}
	}
	return fmt.Errorf("no checksum entry found for %s", assetName)
}

// ExtractBinaryFromArchive extracts the named binary from a .tar.gz or .zip archive byte slice.
func ExtractBinaryFromArchive(archiveBytes []byte, binaryName string, isZip bool) ([]byte, error) {
	if isZip {
		zipReader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
		if err != nil {
			return nil, fmt.Errorf("invalid zip archive: %w", err)
		}
		for _, f := range zipReader.File {
			if filepath.Base(f.Name) == binaryName {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("binary %q not found in zip archive", binaryName)
	}

	// tar.gz
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(header.Name) == binaryName {
			return io.ReadAll(tarReader)
		}
	}
	return nil, fmt.Errorf("binary %q not found in tar.gz archive", binaryName)
}

// ApplyBinaryUpdate replaces the currently running executable with newBinaryBytes in-place.
func ApplyBinaryUpdate(newBinaryBytes []byte) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate running executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	execDir := filepath.Dir(execPath)
	execName := filepath.Base(execPath)

	if runtime.GOOS == "windows" {
		// On Windows, running binaries cannot be overwritten directly.
		// Rename current executable to .old, write the new binary, and let .old be removed.
		oldPath := filepath.Join(execDir, execName+".old")
		_ = os.Remove(oldPath) // Remove any previous leftover

		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to rename running executable on Windows: %w", err)
		}

		if err := os.WriteFile(execPath, newBinaryBytes, 0755); err != nil {
			// Roll back rename if write fails
			_ = os.Rename(oldPath, execPath)
			return fmt.Errorf("failed to write new binary: %w", err)
		}
		return nil
	}

	// Linux / macOS atomic replacement: write to temp file in same directory, then rename
	tmpPath := filepath.Join(execDir, fmt.Sprintf(".%s.new-%d", execName, time.Now().UnixNano()))
	if err := os.WriteFile(tmpPath, newBinaryBytes, 0755); err != nil {
		return fmt.Errorf("failed to write temporary binary: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to swap executable: %w", err)
	}

	return nil
}

// PerformSelfUpdate coordinates downloading the latest release, verifying it, and swapping the current executable.
func (u *UpdateChecker) PerformSelfUpdate(ctx context.Context, currentVersion string, onProgress func(pct int, msg string)) error {
	if onProgress != nil {
		onProgress(10, "Checking for latest release on GitHub...")
	}

	rel, isNewer, err := u.CheckLatest(ctx, currentVersion)
	if err != nil {
		return err
	}
	if !isNewer {
		return fmt.Errorf("already up to date (current: %s, latest: %s)", currentVersion, rel.TagName)
	}

	expectedArchive := TargetArchiveName(rel.TagName, runtime.GOOS, runtime.GOARCH)

	var archiveAsset *ReleaseAsset
	var checksumsAsset *ReleaseAsset

	for i := range rel.Assets {
		asset := &rel.Assets[i]
		if asset.Name == expectedArchive {
			archiveAsset = asset
		} else if asset.Name == "checksums.txt" {
			checksumsAsset = asset
		}
	}

	if archiveAsset == nil {
		return fmt.Errorf("release %s does not contain matching artifact: %s", rel.TagName, expectedArchive)
	}

	if onProgress != nil {
		onProgress(30, fmt.Sprintf("Downloading %s...", expectedArchive))
	}

	archiveBytes, err := u.DownloadAsset(ctx, archiveAsset.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed downloading archive: %w", err)
	}

	if checksumsAsset != nil {
		if onProgress != nil {
			onProgress(60, "Verifying SHA256 checksum...")
		}
		checksumBytes, err := u.DownloadAsset(ctx, checksumsAsset.DownloadURL)
		if err == nil {
			if err := VerifyChecksum(archiveBytes, expectedArchive, string(checksumBytes)); err != nil {
				return fmt.Errorf("security verification failed: %w", err)
			}
		}
	}

	if onProgress != nil {
		onProgress(80, "Extracting binary...")
	}

	binaryName := "waterstick"
	if runtime.GOOS == "windows" {
		binaryName = "waterstick.exe"
	}

	isZip := runtime.GOOS == "windows"
	binaryBytes, err := ExtractBinaryFromArchive(archiveBytes, binaryName, isZip)
	if err != nil {
		return fmt.Errorf("failed extracting binary: %w", err)
	}

	if onProgress != nil {
		onProgress(95, "Applying binary update in-place...")
	}

	if err := ApplyBinaryUpdate(binaryBytes); err != nil {
		return fmt.Errorf("failed applying update: %w", err)
	}

	if onProgress != nil {
		onProgress(100, fmt.Sprintf("Successfully updated to %s", rel.TagName))
	}

	return nil
}

func getGitHubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return ""
}

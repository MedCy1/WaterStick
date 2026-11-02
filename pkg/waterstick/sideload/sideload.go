package sideload

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

//go:embed apps.json
var embeddedAppsJSON []byte

// Catalog manages curated open-source TV applications and installation pipelines.
type Catalog struct {
	client *adb.Client
	db     models.AppDatabase
}

// NewCatalog initializes the sideload catalog with embedded metadata.
func NewCatalog(client *adb.Client) (*Catalog, error) {
	var db models.AppDatabase
	if err := json.Unmarshal(embeddedAppsJSON, &db); err != nil {
		return nil, fmt.Errorf("failed to parse embedded apps.json: %w", err)
	}
	return &Catalog{
		client: client,
		db:     db,
	}, nil
}

// GetApps returns the list of available sideloadable applications.
func (c *Catalog) GetApps() []models.SideloadApp {
	return c.db.Apps
}

// GetApp looks up an application by ID.
func (c *Catalog) GetApp(id string) (*models.SideloadApp, bool) {
	for _, a := range c.db.Apps {
		if a.ID == id {
			return &a, true
		}
	}
	return nil, false
}

// AnnotateInstalled checks which apps are already installed on user 0.
func (c *Catalog) AnnotateInstalled(ctx context.Context, deviceSerial string) ([]models.SideloadApp, error) {
	out, err := c.client.Shell(ctx, deviceSerial, "pm list packages --user 0")
	if err != nil {
		return c.db.Apps, err
	}

	installed := make(map[string]bool)
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "package:") {
			installed[strings.TrimPrefix(l, "package:")] = true
		}
	}

	res := make([]models.SideloadApp, len(c.db.Apps))
	for i, a := range c.db.Apps {
		a.Installed = installed[a.PackageName]
		res[i] = a
	}
	return res, nil
}

// InstallApp downloads the latest official APK and installs it via ADB.
func (c *Catalog) InstallApp(ctx context.Context, deviceSerial, appID string, progressFn func(pct int, msg string)) error {
	app, found := c.GetApp(appID)
	if !found {
		return fmt.Errorf("app with ID %q not found in catalog", appID)
	}

	if progressFn != nil {
		progressFn(5, fmt.Sprintf("Downloading %s from %s...", app.Name, app.DownloadURL))
	}

	tmpDir, err := os.MkdirTemp("", "waterstick-sideload-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	apkPath := filepath.Join(tmpDir, app.ID+".apk")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 WaterStick/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", app.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download server returned HTTP %d for %s", resp.StatusCode, app.Name)
	}

	out, err := os.Create(apkPath)
	if err != nil {
		return err
	}

	totalBytes := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			out.Close()
			return ctx.Err()
		default:
		}

		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				out.Close()
				return wErr
			}
			downloaded += int64(n)
			if totalBytes > 0 && progressFn != nil {
				pct := int((downloaded * 75) / totalBytes)
				progressFn(5+pct, fmt.Sprintf("Downloading %s... (%d MB)", app.Name, downloaded/(1024*1024)))
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			out.Close()
			return rErr
		}
	}
	out.Close()

	if progressFn != nil {
		progressFn(85, fmt.Sprintf("Installing %s onto Fire TV via ADB...", app.Name))
	}

	if err := c.client.InstallAPK(ctx, deviceSerial, apkPath); err != nil {
		return fmt.Errorf("failed installing %s: %w", app.Name, err)
	}

	// Sideloaded apps run interpreted/JIT until Fire OS's idle dexopt applies their baseline
	// profile, which can take days. Compiling it now is what makes Compose UIs smooth from the
	// first launch (WaterOS measured: 32% missed frames before, 0% after). Best effort.
	if progressFn != nil {
		progressFn(92, fmt.Sprintf("Optimizing %s for this device...", app.Name))
	}
	_, _ = c.client.Shell(ctx, deviceSerial, "cmd package compile -m speed-profile -f "+app.PackageName)

	if progressFn != nil {
		progressFn(100, fmt.Sprintf("Successfully installed %s!", app.Name))
	}

	return nil
}

package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v1.0.1", "v1.0.0", 1},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"dev", "v1.0.0", -1},
		{"v1.0.0", "dev", 1},
		{"v1.0.0-rc1", "v1.0.0", 0},
	}

	for _, tt := range tests {
		got := CompareVersions(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestTargetArchiveName(t *testing.T) {
	linuxArchive := TargetArchiveName("v1.0.0", "linux", "amd64")
	if linuxArchive != "waterstick_v1.0.0_linux_amd64.tar.gz" {
		t.Errorf("unexpected linux archive name: %s", linuxArchive)
	}

	windowsArchive := TargetArchiveName("v1.0.0", "windows", "amd64")
	if windowsArchive != "waterstick_v1.0.0_windows_amd64.zip" {
		t.Errorf("unexpected windows archive name: %s", windowsArchive)
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("mock binary content for waterstick")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	checksumsContent := fmt.Sprintf("%s  waterstick_v1.0.0_linux_amd64.tar.gz\n9999999  other.tar.gz\n", hash)

	err := VerifyChecksum(data, "waterstick_v1.0.0_linux_amd64.tar.gz", checksumsContent)
	if err != nil {
		t.Fatalf("expected valid checksum verification, got: %v", err)
	}

	// Corrupt payload test
	errCorrupt := VerifyChecksum([]byte("tampered content"), "waterstick_v1.0.0_linux_amd64.tar.gz", checksumsContent)
	if errCorrupt == nil {
		t.Fatal("expected failure on tampered content, got nil error")
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	binaryContent := []byte("ELF executable binary content")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	header := &tar.Header{
		Name: "waterstick",
		Mode: 0755,
		Size: int64(len(binaryContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatalf("failed to write tar body: %v", err)
	}
	tw.Close()
	gw.Close()

	extracted, err := ExtractBinaryFromArchive(buf.Bytes(), "waterstick", false)
	if err != nil {
		t.Fatalf("failed to extract binary from tar.gz: %v", err)
	}
	if !bytes.Equal(extracted, binaryContent) {
		t.Fatalf("extracted binary content mismatch: got %q, expected %q", string(extracted), string(binaryContent))
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	binaryContent := []byte("PE executable binary content")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create("waterstick.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := f.Write(binaryContent); err != nil {
		t.Fatalf("failed to write zip content: %v", err)
	}
	zw.Close()

	extracted, err := ExtractBinaryFromArchive(buf.Bytes(), "waterstick.exe", true)
	if err != nil {
		t.Fatalf("failed to extract binary from zip: %v", err)
	}
	if !bytes.Equal(extracted, binaryContent) {
		t.Fatalf("extracted binary content mismatch: got %q, expected %q", string(extracted), string(binaryContent))
	}
}

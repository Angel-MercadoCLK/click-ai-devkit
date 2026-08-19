package selfupdate

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxExtractedBytes = 128 * 1024 * 1024 // 128 MiB
)

var (
	// createStageTemp is a seam for testing stage temp file creation
	createStageTemp = func(dir, pattern string) (*os.File, error) {
		return os.CreateTemp(dir, pattern)
	}
)

// extractExecutable validates and extracts the executable from a ZIP archive.
// The ZIP must contain exactly one root-level regular file named "click" or "click.exe".
// Returns the path to the extracted executable or an error.
func extractExecutable(zipReader io.ReaderAt, zipSize int64, stageDir, expectedExeName string) (string, error) {
	// Open the ZIP archive
	r, err := zip.NewReader(zipReader, zipSize)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}

	// Validate ZIP contents and find the executable
	var exeFile *zip.File
	exeCount := 0
	totalFiles := 0

	for _, f := range r.File {
		totalFiles++

		// Validate entry name
		if err := validateZipEntryName(f.Name); err != nil {
			return "", err
		}

		// Check if this is our expected executable
		if f.Name == expectedExeName {
			exeCount++
			if exeCount > 1 {
				return "", fmt.Errorf("duplicate executable entry: %s", expectedExeName)
			}
			exeFile = f
		}
	}

	// Must have exactly one executable and no other files
	if exeCount != 1 {
		return "", fmt.Errorf("expected exactly one executable named %s, found %d", expectedExeName, exeCount)
	}
	if totalFiles != 1 {
		return "", fmt.Errorf("ZIP must contain only the executable, found %d files", totalFiles)
	}

	// Validate the executable entry
	if err := validateExecutableEntry(exeFile); err != nil {
		return "", err
	}

	// Extract the executable
	return extractToFile(exeFile, stageDir)
}

// validateZipEntryName checks that a ZIP entry name is safe.
func validateZipEntryName(name string) error {
	// Reject absolute paths
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute path not allowed: %s", name)
	}

	// Reject path traversal
	if strings.Contains(name, "..") {
		return fmt.Errorf("path traversal not allowed: %s", name)
	}

	// Reject empty names
	if name == "" {
		return errors.New("empty entry name")
	}

	return nil
}

// validateExecutableEntry validates that a ZIP entry is a valid executable.
func validateExecutableEntry(f *zip.File) error {
	// Check it's a regular file (not directory or symlink)
	if f.Mode().IsDir() {
		return fmt.Errorf("entry is a directory: %s", f.Name)
	}
	if f.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("entry is a symlink: %s", f.Name)
	}

	// Check uncompressed size
	if f.UncompressedSize64 > maxExtractedBytes {
		return fmt.Errorf("executable too large: %d bytes exceeds %d byte limit", f.UncompressedSize64, maxExtractedBytes)
	}

	return nil
}

// extractToFile extracts a ZIP file to the stage directory.
func extractToFile(f *zip.File, stageDir string) (string, error) {
	// Open the compressed file
	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("open zip entry: %w", err)
	}
	defer rc.Close()

	// Create temp file in stage directory, preserving the original filename
	tempFile, err := createStageTemp(stageDir, "stage-*"+filepath.Ext(f.Name))
	if err != nil {
		return "", fmt.Errorf("create stage temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Ensure cleanup on error
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tempPath)
		}
	}()

	// Copy the executable content
	if _, err := io.Copy(tempFile, io.LimitReader(rc, maxExtractedBytes+1)); err != nil {
		_ = tempFile.Close()
		return "", fmt.Errorf("extract executable: %w", err)
	}

	// Sync and close the file
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return "", fmt.Errorf("sync stage file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close stage file: %w", err)
	}

	// Make the file executable
	if err := os.Chmod(tempPath, 0755); err != nil {
		return "", fmt.Errorf("make executable: %w", err)
	}

	success = true
	return tempPath, nil
}

// PreparedRelease represents a release that has been downloaded and verified.
type PreparedRelease struct {
	// ArchivePath is the path to the downloaded archive.
	ArchivePath string
	// StagePath is the path to the extracted executable.
	StagePath string
	// Digest is the SHA-256 digest of the downloaded archive.
	Digest []byte
	// ExpectedDigest is the SHA-256 digest from checksums.txt.
	ExpectedDigest string
	// cleanup removes the temporary files.
	cleanup func()
}

// prepareRelease downloads, verifies, and extracts a release archive.
// It enforces the order: fetch checksums → download archive → verify digest → extract.
// Returns a PreparedRelease or an error. The caller must call cleanup() when done.
func prepareRelease(asset ReleaseAsset, targetDir string) (*PreparedRelease, error) {
	// Step 1: Fetch checksums.txt
	// The checksums.txt is in the same directory as the archive
	// Extract the base URL from the archive URL
	archiveURL := asset.ArchiveURL
	lastSlash := strings.LastIndex(archiveURL, "/")
	if lastSlash == -1 {
		return nil, fmt.Errorf("invalid archive URL: %s", archiveURL)
	}
	baseURL := archiveURL[:lastSlash+1]
	checksumsURL := baseURL + "checksums.txt"

	checksumsData, err := fetchChecksums(checksumsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch checksums: %w", err)
	}

	// Step 2: Parse expected checksum for this archive
	expectedDigest, err := expectedChecksum(bytes.NewReader(checksumsData), asset.ArchiveName)
	if err != nil {
		return nil, fmt.Errorf("parse checksums: %w", err)
	}

	// Step 3: Download archive to temp file
	tempPath, digest, err := downloadToSibling(asset.ArchiveURL, targetDir, "download-*.zip")
	if err != nil {
		return nil, fmt.Errorf("download archive: %w", err)
	}

	// Step 4: Verify digest matches expected
	digestHex := hex.EncodeToString(digest)
	if digestHex != expectedDigest {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("digest mismatch: computed %s, expected %s", digestHex, expectedDigest)
	}

	// Step 5: Extract executable from ZIP
	// Open the downloaded ZIP file
	zipFile, err := os.Open(tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("open downloaded archive: %w", err)
	}
	defer zipFile.Close()

	// Get file info for size
	fileInfo, err := zipFile.Stat()
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("stat archive: %w", err)
	}

	stagePath, err := extractExecutable(zipFile, fileInfo.Size(), targetDir, asset.ExecutableName)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("extract executable: %w", err)
	}

	// Create cleanup function
	cleanup := func() {
		_ = os.Remove(tempPath)
		if stagePath != "" {
			_ = os.Remove(stagePath)
		}
	}

	return &PreparedRelease{
		ArchivePath:    tempPath,
		StagePath:      stagePath,
		Digest:         digest,
		ExpectedDigest: expectedDigest,
		cleanup:        cleanup,
	}, nil
}

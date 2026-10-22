package installer

import (
	"archive/zip"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// parseDotChecksum returns the SHA-256 published for asset in a goreleaser-style checksums.txt
// ("<sha256>  <name>"). A missing, duplicated or malformed entry is an error: the archive is only
// ever trusted when the same release states exactly one digest for it. This establishes integrity,
// not authenticity; TLS to GitHub is the authenticity boundary (same stance as the self-update path).
func parseDotChecksum(data []byte, asset string) ([]byte, error) {
	var found []byte
	for number, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("checksums.txt tiene una línea mal formada (%d)", number+1)
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != 32 {
			return nil, fmt.Errorf("checksums.txt tiene un SHA-256 no válido en la línea %d", number+1)
		}
		if strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("checksums.txt lista %s más de una vez", asset)
		}
		found = digest
	}
	if found == nil {
		return nil, fmt.Errorf("checksums.txt no incluye %s; se rechazó la descarga", asset)
	}
	return found, nil
}

// extractDotArchive unpacks a verified zip into dest (which must not exist). Every entry is validated
// before any byte is written for it: no absolute or drive-qualified names, no ".." components, no
// symlinks or other special files, no duplicate names (case-insensitively, like Windows), and a hard
// budget on entries and actual decompressed bytes (headers are never trusted for sizing).
func extractDotArchive(archive, dest string, maxBytes int64, maxEntries int) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) > maxEntries {
		return fmt.Errorf("demasiadas entradas (máximo %d)", maxEntries)
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}
	root := filepath.Clean(dest)
	seen := make(map[string]struct{}, len(reader.File))
	remaining := maxBytes
	for _, entry := range reader.File {
		name, err := cleanDotEntryName(entry.Name)
		if err != nil {
			return err
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("entrada duplicada: %s", entry.Name)
		}
		seen[key] = struct{}{}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !mode.IsRegular()) {
			return fmt.Errorf("entrada no permitida (enlace o archivo especial): %s", entry.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if rel, err := filepath.Rel(root, target); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("entrada fuera del destino: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if entry.UncompressedSize64 > uint64(remaining) {
			return errors.New("el contenido descomprimido supera el tamaño máximo permitido")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		written, err := writeDotEntry(entry, target, remaining)
		if err != nil {
			return err
		}
		remaining -= written
	}
	return nil
}

func writeDotEntry(entry *zip.File, target string, budget int64) (int64, error) {
	source, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer source.Close()
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, budget+1))
	closeErr := file.Close()
	switch {
	case copyErr != nil:
		return 0, copyErr
	case written > budget:
		return 0, errors.New("el contenido descomprimido supera el tamaño máximo permitido")
	}
	return written, closeErr
}

// cleanDotEntryName normalizes a zip entry name (tolerating the backslash separators some Windows
// archivers write) and rejects anything that could resolve outside the destination.
func cleanDotEntryName(raw string) (string, error) {
	name := strings.TrimSuffix(strings.ReplaceAll(raw, `\`, "/"), "/")
	if name == "" || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("nombre de entrada no válido: %q", raw)
	}
	for _, part := range strings.Split(name, "/") {
		switch {
		case part == "" || part == "." || part == "..":
			return "", fmt.Errorf("nombre de entrada no válido: %q", raw)
		case strings.ContainsAny(part, ":*?\"<>|") || strings.ContainsFunc(part, func(r rune) bool { return r < 0x20 }):
			return "", fmt.Errorf("nombre de entrada no válido: %q", raw)
		case strings.HasSuffix(part, ".") || strings.HasSuffix(part, " "):
			return "", fmt.Errorf("nombre de entrada no válido: %q", raw)
		}
	}
	return name, nil
}

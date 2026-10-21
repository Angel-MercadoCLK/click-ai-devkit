package installer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const testDotVersion = "1.2.3"

type zipEntry struct {
	name    string
	body    string
	mode    os.FileMode
	isDir   bool
	symlink bool
}

func buildZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		switch {
		case e.isDir:
			header.SetMode(os.ModeDir | 0o755)
		case e.symlink:
			header.SetMode(os.ModeSymlink | 0o777)
		case e.mode != 0:
			header.SetMode(e.mode)
		default:
			header.SetMode(0o644)
		}
		fw, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if !e.isDir {
			if _, err := fw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func validDotZip(t *testing.T, label string) []byte {
	return buildZip(t,
		zipEntry{name: "Click Dot.exe", body: "exe-" + label},
		zipEntry{name: "resources/app.js", body: "js-" + label},
	)
}

func checksumsFor(asset string, archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:]) + "  " + asset + "\n" + strings.Repeat("0", 64) + "  click_other.zip\n"
}

type dotHarness struct {
	t       *testing.T
	manager DotManager
	local   string
	roaming string
	files   map[string][]byte // url -> body
	fetched []string

	mu       sync.Mutex
	runs     [][]string
	started  []string
	runError func(name string, args []string) error
	startErr error
	login    *fakeLoginStore
}

type fakeLoginStore struct {
	entries map[string]string
	deleted []string
}

func (f *fakeLoginStore) Entries() (map[string]string, error) { return f.entries, nil }
func (f *fakeLoginStore) Delete(name string) error {
	f.deleted = append(f.deleted, name)
	delete(f.entries, name)
	return nil
}

func newDotHarness(t *testing.T) *dotHarness {
	t.Helper()
	root := t.TempDir()
	h := &dotHarness{t: t, local: filepath.Join(root, "Local"), roaming: filepath.Join(root, "Roaming"), files: map[string][]byte{}, login: &fakeLoginStore{entries: map[string]string{}}}
	env := map[string]string{"LOCALAPPDATA": h.local, "APPDATA": h.roaming, "SystemRoot": filepath.Join(root, "Windows")}
	h.manager = DotManager{
		GOOS: "windows", GOARCH: "amd64",
		Getenv:  func(k string) string { return env[k] },
		BaseURL: "https://example.test/releases/download",
		Fetch: func(_ context.Context, url string) (io.ReadCloser, error) {
			h.fetched = append(h.fetched, url)
			body, ok := h.files[url]
			if !ok {
				return nil, errors.New("HTTP 404")
			}
			return io.NopCloser(bytes.NewReader(body)), nil
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			h.mu.Lock()
			h.runs = append(h.runs, append([]string{name}, args...))
			h.mu.Unlock()
			if h.runError != nil {
				return nil, h.runError(name, args)
			}
			return nil, nil
		},
		Start: func(exe string) error {
			h.started = append(h.started, exe)
			return h.startErr
		},
		LoginItems: h.login,
		Sleep:      func(time.Duration) {},
	}
	return h
}

func (h *dotHarness) release(version string, archive []byte) {
	base := "https://example.test/releases/download/v" + version + "/"
	asset := DotAssetName(version)
	h.files[base+asset] = archive
	h.files[base+"checksums.txt"] = []byte(checksumsFor(asset, archive))
}

func (h *dotHarness) installDir() string { return filepath.Join(h.local, "Programs", "Click Dot") }
func (h *dotHarness) shortcut() string {
	return filepath.Join(h.roaming, "Microsoft", "Windows", "Start Menu", "Programs", "Click Dot.lnk")
}

func (h *dotHarness) runNames() []string {
	var names []string
	for _, r := range h.runs {
		names = append(names, filepath.Base(r[0]))
	}
	return names
}

func readDotFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDotReleaseVersionRejectsDevelopmentBuilds(t *testing.T) {
	for in, want := range map[string]string{
		"1.2.3": "1.2.3", "v1.2.3": "1.2.3", "1.2.3-rc.1": "1.2.3-rc.1",
		"dev": "", "": "", "1.2": "", "1.2.3-SNAPSHOT-abc": "", "1.2.3/../x": "", "1.2.3 ": "1.2.3",
	} {
		if got := DotReleaseVersion(in); got != want {
			t.Errorf("DotReleaseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDotAssetNameMatchesReleaseContract(t *testing.T) {
	if got := DotAssetName("1.2.3"); got != "click-dot_1.2.3_windows_amd64.zip" {
		t.Fatalf("asset name = %q", got)
	}
}

func TestDotSyncSkipsOffWindowsWithoutNetworkOrWrites(t *testing.T) {
	h := newDotHarness(t)
	for _, goos := range []string{"linux", "darwin"} {
		h.manager.GOOS = goos
		res, err := h.manager.Sync(context.Background(), testDotVersion)
		if err != nil || res.Outcome != DotSkippedPlatform {
			t.Fatalf("%s: res=%+v err=%v", goos, res, err)
		}
	}
	h.manager.GOOS, h.manager.GOARCH = "windows", "arm64"
	if res, _ := h.manager.Sync(context.Background(), testDotVersion); res.Outcome != DotSkippedPlatform {
		t.Fatalf("windows/arm64 must skip, got %+v", res)
	}
	if len(h.fetched) != 0 || len(h.runs) != 0 {
		t.Fatalf("skip must not touch network or run commands: fetched=%v runs=%v", h.fetched, h.runs)
	}
	if _, err := os.Stat(h.local); !os.IsNotExist(err) {
		t.Fatalf("skip must not write the filesystem: %v", err)
	}
}

func TestDotSyncSkipsDevelopmentBuild(t *testing.T) {
	h := newDotHarness(t)
	res, err := h.manager.Sync(context.Background(), "dev")
	if err != nil || res.Outcome != DotSkippedDev {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(h.fetched) != 0 || len(h.runs) != 0 {
		t.Fatal("dev build must not download or run anything")
	}
}

func TestDotPlanIsPure(t *testing.T) {
	h := newDotHarness(t)
	plan := h.manager.Plan("v" + testDotVersion)
	if plan.Outcome != DotInstalled || plan.Version != testDotVersion || plan.Asset != "click-dot_1.2.3_windows_amd64.zip" || plan.InstallDir != h.installDir() {
		t.Fatalf("plan = %+v", plan)
	}
	if h.manager.Plan("dev").Outcome != DotSkippedDev {
		t.Fatal("dev plan must skip")
	}
	h.manager.GOOS = "linux"
	if h.manager.Plan(testDotVersion).Outcome != DotSkippedPlatform {
		t.Fatal("linux plan must skip")
	}
	if len(h.fetched) != 0 || len(h.runs) != 0 {
		t.Fatal("plan must not touch network or commands")
	}
	if _, err := os.Stat(h.local); !os.IsNotExist(err) {
		t.Fatal("plan must not write")
	}
}

func TestDotSyncInstallsVerifiedPackage(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))

	res, err := h.manager.Sync(context.Background(), testDotVersion)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != DotInstalled || res.Version != testDotVersion || len(res.Warnings) != 0 {
		t.Fatalf("res = %+v", res)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-one" {
		t.Fatalf("exe = %q", got)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "resources", "app.js")); got != "js-one" {
		t.Fatalf("nested file = %q", got)
	}
	if got := strings.TrimSpace(readDotFile(t, filepath.Join(h.installDir(), ".click-dot-version"))); got != testDotVersion {
		t.Fatalf("marker = %q", got)
	}
	wantURL := "https://example.test/releases/download/v1.2.3/click-dot_1.2.3_windows_amd64.zip"
	if len(h.fetched) != 2 || !strings.HasSuffix(h.fetched[0], "/v1.2.3/checksums.txt") || h.fetched[1] != wantURL {
		t.Fatalf("fetched = %v", h.fetched)
	}
	if got := h.runNames(); len(got) != 2 || got[0] != "taskkill.exe" || got[1] != "powershell.exe" {
		t.Fatalf("commands = %v (want taskkill then powershell)", got)
	}
	taskkill := h.runs[0]
	if strings.Join(taskkill[1:], " ") != "/IM Click Dot.exe /F" {
		t.Fatalf("taskkill args = %v", taskkill[1:])
	}
	script := h.runs[1][len(h.runs[1])-1]
	for _, want := range []string{"WScript.Shell", "'" + h.shortcut() + "'", "'" + filepath.Join(h.installDir(), "Click Dot.exe") + "'"} {
		if !strings.Contains(script, want) {
			t.Fatalf("shortcut script missing %q:\n%s", want, script)
		}
	}
	if len(h.started) != 1 || h.started[0] != filepath.Join(h.installDir(), "Click Dot.exe") {
		t.Fatalf("started = %v", h.started)
	}
	entries, _ := os.ReadDir(filepath.Join(h.local, "Programs"))
	if len(entries) != 1 || entries[0].Name() != "Click Dot" {
		t.Fatalf("staging/backup directories must be cleaned up, found %v", entries)
	}
}

func TestDotSyncUpdatesReplacingPreviousVersion(t *testing.T) {
	h := newDotHarness(t)
	h.release("1.2.2", validDotZip(t, "old"))
	h.release(testDotVersion, validDotZip(t, "new"))
	if _, err := h.manager.Sync(context.Background(), "1.2.2"); err != nil {
		t.Fatal(err)
	}
	// A stale file from the previous version must not survive the replacement.
	if err := os.WriteFile(filepath.Join(h.installDir(), "stale.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.manager.Sync(context.Background(), testDotVersion)
	if err != nil || res.Outcome != DotUpdated {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-new" {
		t.Fatalf("exe = %q", got)
	}
	if _, err := os.Stat(filepath.Join(h.installDir(), "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("stale file survived the atomic replacement")
	}
}

func TestDotSyncSkipsDownloadWhenAlreadyCurrent(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err != nil {
		t.Fatal(err)
	}
	h.fetched, h.runs, h.started = nil, nil, nil
	res, err := h.manager.Sync(context.Background(), testDotVersion)
	if err != nil || res.Outcome != DotCurrent {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(h.fetched) != 0 {
		t.Fatalf("current install must not download: %v", h.fetched)
	}
	if len(h.started) != 0 {
		t.Fatal("current install must not restart a running Dot")
	}
	if got := h.runNames(); len(got) != 1 || got[0] != "powershell.exe" {
		t.Fatalf("only the shortcut refresh may run, got %v", got)
	}
}

func TestDotSyncReinstallsWhenExecutableMissing(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(h.installDir(), "Click Dot.exe")); err != nil {
		t.Fatal(err)
	}
	res, err := h.manager.Sync(context.Background(), testDotVersion)
	if err != nil || res.Outcome != DotUpdated {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestDotSyncRejectsChecksumMismatchLeavingInstallUntouched(t *testing.T) {
	h := newDotHarness(t)
	h.release("1.2.2", validDotZip(t, "old"))
	if _, err := h.manager.Sync(context.Background(), "1.2.2"); err != nil {
		t.Fatal(err)
	}
	h.release(testDotVersion, validDotZip(t, "new"))
	base := "https://example.test/releases/download/v" + testDotVersion + "/"
	h.files[base+DotAssetName(testDotVersion)] = validDotZip(t, "tampered")
	h.runs, h.started = nil, nil

	_, err := h.manager.Sync(context.Background(), testDotVersion)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("err = %v", err)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-old" {
		t.Fatalf("install must be untouched, exe = %q", got)
	}
	if len(h.runs) != 0 || len(h.started) != 0 {
		t.Fatal("a rejected download must not stop or start Dot")
	}
	entries, _ := os.ReadDir(filepath.Join(h.local, "Programs"))
	if len(entries) != 1 {
		t.Fatalf("staging not cleaned: %v", entries)
	}
}

func TestDotSyncRejectsMissingChecksumEntry(t *testing.T) {
	h := newDotHarness(t)
	archive := validDotZip(t, "one")
	h.release(testDotVersion, archive)
	base := "https://example.test/releases/download/v" + testDotVersion + "/"
	h.files[base+"checksums.txt"] = []byte(strings.Repeat("a", 64) + "  some_other_asset.zip\n")

	_, err := h.manager.Sync(context.Background(), testDotVersion)
	if err == nil || !strings.Contains(err.Error(), "no incluye") {
		t.Fatalf("err = %v", err)
	}
	if len(h.fetched) != 1 {
		t.Fatalf("the archive must not be downloaded without a checksum entry: %v", h.fetched)
	}
	if _, statErr := os.Stat(h.installDir()); !os.IsNotExist(statErr) {
		t.Fatal("nothing may be installed")
	}
}

func TestDotSyncRejectsOversizedArchive(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))
	h.manager.MaxArchiveBytes = 16
	_, err := h.manager.Sync(context.Background(), testDotVersion)
	if err == nil || !strings.Contains(err.Error(), "tamaño máximo") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(h.installDir()); !os.IsNotExist(statErr) {
		t.Fatal("nothing may be installed")
	}
}

func TestDotSyncRejectsPackageWithoutExecutable(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, buildZip(t, zipEntry{name: "readme.txt", body: "x"}))
	_, err := h.manager.Sync(context.Background(), testDotVersion)
	if err == nil || !strings.Contains(err.Error(), "Click Dot.exe") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(h.installDir()); !os.IsNotExist(statErr) {
		t.Fatal("nothing may be installed")
	}
}

func TestDotSyncRejectsHostileArchives(t *testing.T) {
	cases := map[string][]zipEntry{
		"zip-slip":         {{name: "../evil.txt", body: "x"}, {name: "Click Dot.exe", body: "e"}},
		"nested zip-slip":  {{name: "a/../../evil.txt", body: "x"}},
		"backslash slip":   {{name: `..\evil.txt`, body: "x"}},
		"absolute path":    {{name: "/etc/evil", body: "x"}},
		"drive path":       {{name: `C:\evil.txt`, body: "x"}},
		"symlink":          {{name: "Click Dot.exe", body: "e"}, {name: "link", body: "/etc/passwd", symlink: true}},
		"duplicate":        {{name: "Click Dot.exe", body: "a"}, {name: "click dot.exe", body: "b"}},
		"alternate stream": {{name: "Click Dot.exe", body: "e"}, {name: "a.txt:ads", body: "x"}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			h := newDotHarness(t)
			h.release(testDotVersion, buildZip(t, entries...))
			if _, err := h.manager.Sync(context.Background(), testDotVersion); err == nil {
				t.Fatal("hostile archive was accepted")
			}
			if _, err := os.Stat(h.installDir()); !os.IsNotExist(err) {
				t.Fatal("nothing may be installed")
			}
			if _, err := os.Stat(filepath.Join(h.local, "evil.txt")); !os.IsNotExist(err) {
				t.Fatal("file escaped the staging directory")
			}
			if _, err := os.Stat(filepath.Join(h.local, "Programs", "evil.txt")); !os.IsNotExist(err) {
				t.Fatal("file escaped the staging directory")
			}
		})
	}
}

func TestDotSyncRejectsDecompressionBomb(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, buildZip(t,
		zipEntry{name: "Click Dot.exe", body: "e"},
		zipEntry{name: "big.bin", body: strings.Repeat("A", 4096)},
	))
	h.manager.MaxExtractedBytes = 1024
	_, err := h.manager.Sync(context.Background(), testDotVersion)
	if err == nil || !strings.Contains(err.Error(), "tamaño máximo") {
		t.Fatalf("err = %v", err)
	}
}

func TestDotSyncRejectsTooManyEntries(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, buildZip(t,
		zipEntry{name: "Click Dot.exe", body: "e"}, zipEntry{name: "a", body: "1"}, zipEntry{name: "b", body: "2"},
	))
	h.manager.MaxEntries = 2
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err == nil {
		t.Fatal("entry budget not enforced")
	}
}

func TestDotSyncDownloadFailureReportsAndWritesNothing(t *testing.T) {
	h := newDotHarness(t) // no release published: every fetch is a 404
	_, err := h.manager.Sync(context.Background(), testDotVersion)
	if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(h.installDir()); !os.IsNotExist(statErr) {
		t.Fatal("nothing may be installed")
	}
}

func TestDotSyncRestoresPreviousInstallWhenSwapFails(t *testing.T) {
	h := newDotHarness(t)
	h.release("1.2.2", validDotZip(t, "old"))
	h.release(testDotVersion, validDotZip(t, "new"))
	if _, err := h.manager.Sync(context.Background(), "1.2.2"); err != nil {
		t.Fatal(err)
	}
	// Make the final rename fail by pre-creating nothing but removing write access is
	// platform-dependent; exercise the helper directly instead.
	next := filepath.Join(t.TempDir(), "does-not-exist")
	if err := h.manager.swapInstallDir(h.installDir(), next); err == nil {
		t.Fatal("expected swap failure")
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-old" {
		t.Fatalf("previous install must be restored, exe = %q", got)
	}
	entries, _ := os.ReadDir(filepath.Join(h.local, "Programs"))
	if len(entries) != 1 {
		t.Fatalf("aside directory leaked: %v", entries)
	}
}

func TestDotSwapRetriesThenKeepsInstallWhenDirectoryStaysLocked(t *testing.T) {
	if os.Geteuid() <= 0 || runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced for root or on Windows")
	}
	h := newDotHarness(t)
	h.release("1.2.2", validDotZip(t, "old"))
	if _, err := h.manager.Sync(context.Background(), "1.2.2"); err != nil {
		t.Fatal(err)
	}
	sleeps := 0
	h.manager.Sleep = func(time.Duration) { sleeps++ }
	// A read-only parent makes the aside rename fail on every attempt, like a locked directory.
	programs := filepath.Join(h.local, "Programs")
	if err := os.Chmod(programs, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(programs, 0o755) })
	next := filepath.Join(t.TempDir(), "next")
	if err := os.MkdirAll(next, 0o755); err != nil {
		t.Fatal(err)
	}
	err := h.manager.swapInstallDir(h.installDir(), next)
	if err == nil || !strings.Contains(err.Error(), "Click Dot sigue abierto") {
		t.Fatalf("err = %v", err)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-old" {
		t.Fatalf("install must stay intact, exe = %q", got)
	}
	if sleeps != dotSwapAttempts {
		t.Fatalf("sleeps = %d, want %d retries", sleeps, dotSwapAttempts)
	}
}

func TestDotSyncShortcutAndStartFailuresAreWarnings(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))
	h.runError = func(name string, args []string) error {
		if strings.HasSuffix(name, "powershell.exe") {
			return errors.New("boom")
		}
		return nil
	}
	h.startErr = errors.New("no arranca")
	res, err := h.manager.Sync(context.Background(), testDotVersion)
	if err != nil || res.Outcome != DotInstalled {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(res.Warnings) != 2 || !strings.Contains(res.Warnings[0], "acceso directo") || !strings.Contains(res.Warnings[1], "no se pudo iniciar") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-one" {
		t.Fatal("install must survive shortcut/start failures")
	}
}

func TestDotSyncToleratesTaskkillNotRunning(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))
	h.runError = func(name string, args []string) error {
		if strings.HasSuffix(name, "taskkill.exe") {
			return errors.New("exit status 128")
		}
		return nil
	}
	if res, err := h.manager.Sync(context.Background(), testDotVersion); err != nil || res.Outcome != DotInstalled {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestDotSyncRequiresAbsoluteUserDirectories(t *testing.T) {
	h := newDotHarness(t)
	h.manager.Getenv = func(k string) string {
		if k == "LOCALAPPDATA" {
			return "relative/dir"
		}
		return h.roaming
	}
	h.release(testDotVersion, validDotZip(t, "one"))
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err == nil || !strings.Contains(err.Error(), "LOCALAPPDATA") {
		t.Fatalf("err = %v", err)
	}
	if len(h.fetched) != 0 {
		t.Fatal("must fail before any download")
	}
}

func TestDotInstallsOnlyUnderPerUserPrograms(t *testing.T) {
	h := newDotHarness(t)
	h.release(testDotVersion, validDotZip(t, "one"))
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err != nil {
		t.Fatal(err)
	}
	var written []string
	_ = filepath.WalkDir(filepath.Dir(h.local), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			written = append(written, path)
		}
		return nil
	})
	for _, path := range written {
		if !strings.HasPrefix(path, h.installDir()+string(filepath.Separator)) {
			t.Fatalf("file written outside the install directory: %s", path)
		}
	}
}

func TestDotInspect(t *testing.T) {
	h := newDotHarness(t)
	if s := h.manager.Inspect(); !s.Supported || s.ExePresent || s.MarkerPresent || s.Version != "" {
		t.Fatalf("empty = %+v", s)
	}
	h.release(testDotVersion, validDotZip(t, "one"))
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err != nil {
		t.Fatal(err)
	}
	s := h.manager.Inspect()
	if !s.ExePresent || !s.MarkerPresent || s.Version != testDotVersion || s.ExePath != filepath.Join(h.installDir(), "Click Dot.exe") {
		t.Fatalf("installed = %+v", s)
	}
	h.manager.GOOS = "linux"
	if h.manager.Inspect().Supported {
		t.Fatal("linux is unsupported")
	}
}

func seedInstalledDot(t *testing.T, h *dotHarness) {
	t.Helper()
	h.release(testDotVersion, validDotZip(t, "one"))
	if _, err := h.manager.Sync(context.Background(), testDotVersion); err != nil {
		t.Fatal(err)
	}
	h.runs, h.started = nil, nil
}

func TestDotRemoveDeletesOwnedPathsAndPreservesUserData(t *testing.T) {
	h := newDotHarness(t)
	seedInstalledDot(t, h)
	if err := os.MkdirAll(filepath.Dir(h.shortcut()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.shortcut(), []byte("lnk"), 0o644); err != nil {
		t.Fatal(err)
	}
	userData := filepath.Join(h.roaming, "Click Dot")
	if err := os.MkdirAll(userData, 0o755); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(userData, "credentials.bin")
	if err := os.WriteFile(credentials, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(h.installDir(), "Click Dot.exe")
	h.login.entries = map[string]string{
		"electron.app.Electron": `"` + exe + `" --hidden`,
		"OtherApp":              `"C:\Other\other.exe"`,
	}
	unrelated := filepath.Join(h.local, "Programs", "Other App")
	if err := os.MkdirAll(unrelated, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := h.manager.Remove(context.Background())
	if err != nil || res.Outcome != DotRemoved {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	for _, gone := range []string{h.installDir(), h.shortcut()} {
		if _, err := os.Lstat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s must be removed", gone)
		}
	}
	if readDotFile(t, credentials) != "secret" {
		t.Fatal("Dot user data must be preserved")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("unrelated Programs entries must be preserved")
	}
	if len(h.login.deleted) != 1 || h.login.deleted[0] != "electron.app.Electron" {
		t.Fatalf("deleted login items = %v (only Dot's own may go)", h.login.deleted)
	}
	if _, ok := h.login.entries["OtherApp"]; !ok {
		t.Fatal("foreign Run entry removed")
	}
	if got := h.runNames(); len(got) != 1 || got[0] != "taskkill.exe" {
		t.Fatalf("commands = %v", got)
	}
}

func TestDotRemoveToleratesAbsence(t *testing.T) {
	h := newDotHarness(t)
	res, err := h.manager.Remove(context.Background())
	if err != nil || res.Outcome != DotNotInstalled {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestDotRemoveKeepsDirectoryWithoutClickMarker(t *testing.T) {
	h := newDotHarness(t)
	if err := os.MkdirAll(h.installDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(h.installDir(), "Click Dot.exe")
	if err := os.WriteFile(foreign, []byte("hand-installed"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.manager.Remove(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if readDotFile(t, foreign) != "hand-installed" {
		t.Fatal("a directory without the Click marker is not Click-owned and must be kept")
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "marcador") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestDotRemoveSkipsOffWindows(t *testing.T) {
	h := newDotHarness(t)
	h.manager.GOOS = "linux"
	res, err := h.manager.Remove(context.Background())
	if err != nil || res.Outcome != DotSkippedPlatform || len(h.runs) != 0 {
		t.Fatalf("res=%+v err=%v runs=%v", res, err, h.runs)
	}
}

func TestDotRemoveJoinsStepErrors(t *testing.T) {
	h := newDotHarness(t)
	seedInstalledDot(t, h)
	h.manager.LoginItems = errLoginStore{}
	res, err := h.manager.Remove(context.Background())
	if err == nil || !strings.Contains(err.Error(), "inicio con Windows") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Lstat(h.installDir()); !os.IsNotExist(statErr) {
		t.Fatal("a login-item failure must not stop the directory removal")
	}
	if res.Outcome != DotRemoved {
		t.Fatalf("res = %+v", res)
	}
}

type errLoginStore struct{}

func (errLoginStore) Entries() (map[string]string, error) { return nil, errors.New("registry locked") }
func (errLoginStore) Delete(string) error                 { return nil }

func TestDotLoginCommandMatching(t *testing.T) {
	exe := `C:\Users\Ana\AppData\Local\Programs\Click Dot\Click Dot.exe`
	for command, want := range map[string]bool{
		`"` + exe + `"`:                       true,
		`"` + exe + `" --hidden`:              true,
		`"` + strings.ToLower(exe) + `" --x`:  true,
		exe:                                   true,
		exe + ` --hidden`:                     true,
		`"C:\Other\Click Dot.exe"`:            false,
		exe + `.bak`:                          false,
		`"` + exe:                             false,
		`C:\Users\Ana\AppData\Local\Programs`: false,
		`"C:\Users\Ana\AppData\Local\Programs\Click Dot\Click Dot.exe.old"`: false,
	} {
		if got := dotCommandLaunches(command, exe); got != want {
			t.Errorf("dotCommandLaunches(%q) = %v, want %v", command, got, want)
		}
	}
}

func TestParseDotChecksum(t *testing.T) {
	good := strings.Repeat("ab", 32)
	if sum, err := parseDotChecksum([]byte("# c\n"+good+"  a.zip\n"+good+" *b.zip\n"), "b.zip"); err != nil || hex.EncodeToString(sum) != good {
		t.Fatalf("sum=%x err=%v", sum, err)
	}
	for name, data := range map[string]string{
		"missing":   good + "  b.zip\n",
		"duplicate": good + "  a.zip\n" + good + "  a.zip\n",
		"short":     "abcd  a.zip\n",
		"not hex":   strings.Repeat("zz", 32) + "  a.zip\n",
		"malformed": "just-one-field\n",
		"empty":     "",
	} {
		if _, err := parseDotChecksum([]byte(data), "a.zip"); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestFetchDotAssetRequiresHTTPSFromGitHub(t *testing.T) {
	for _, bad := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x", "ftp://github.com/x"} {
		if _, err := fetchDotAsset(context.Background(), bad); err == nil {
			t.Errorf("%s must be rejected", bad)
		}
	}
	for host, want := range map[string]bool{"github.com": true, "release-assets.githubusercontent.com": true, "objects.githubusercontent.com": true, "githubusercontent.com": true, "notgithub.com": false, "github.com.evil": false} {
		if dotHostAllowed(host) != want {
			t.Errorf("dotHostAllowed(%q) != %v", host, want)
		}
	}
}

func TestDotSyncAgainstHTTPReleaseServer(t *testing.T) {
	archive := validDotZip(t, "served")
	asset := DotAssetName(testDotVersion)
	mux := http.NewServeMux()
	mux.HandleFunc("/v"+testDotVersion+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, checksumsFor(asset, archive))
	})
	mux.HandleFunc("/v"+testDotVersion+"/"+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	server := httptest.NewServer(mux)
	defer server.Close()

	h := newDotHarness(t)
	h.manager.BaseURL = server.URL
	h.manager.Fetch = func(ctx context.Context, url string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
	res, err := h.manager.Sync(context.Background(), testDotVersion)
	if err != nil || res.Outcome != DotInstalled {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got := readDotFile(t, filepath.Join(h.installDir(), "Click Dot.exe")); got != "exe-served" {
		t.Fatalf("exe = %q", got)
	}
}

func TestDotPackageFactoryIsReplaceable(t *testing.T) {
	h := newDotHarness(t)
	defer SetDotManagerFactoryForTests(func() DotManager { return h.manager })()
	if !DotSupported() {
		t.Fatal("factory must drive DotSupported")
	}
	if PlanDot("dev").Outcome != DotSkippedDev {
		t.Fatal("factory must drive PlanDot")
	}
	if InspectDot().InstallDir != h.installDir() {
		t.Fatal("factory must drive InspectDot")
	}
}

func TestDotSyncNeverReplacesInstallPathWithoutClickMarker(t *testing.T) {
	for name, seed := range map[string]func(t *testing.T, path string){
		"foreign directory": func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "user-notes.txt"), []byte("mine"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"hand-unzipped Dot without marker": func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "Click Dot.exe"), []byte("manual"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"plain file": func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("not a folder"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newDotHarness(t)
			h.release(testDotVersion, validDotZip(t, "one"))
			seed(t, h.installDir())
			before := snapshotDotTree(t, h.installDir())

			res, err := h.manager.Sync(context.Background(), testDotVersion)
			if err == nil {
				t.Fatalf("Sync must refuse a path Click does not own, got %+v", res)
			}
			if after := snapshotDotTree(t, h.installDir()); after != before {
				t.Fatalf("foreign content changed:\nbefore %s\nafter  %s", before, after)
			}
			if len(h.fetched) != 0 || len(h.runs) != 0 || len(h.started) != 0 {
				t.Fatalf("nothing may be downloaded, stopped or started: fetched=%v runs=%v started=%v", h.fetched, h.runs, h.started)
			}
		})
	}
}

// snapshotDotTree renders every path and file content under root (or root itself when it is a file).
func snapshotDotTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(rel)
		if !d.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			b.WriteString("=" + string(data))
		}
		b.WriteString(";")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

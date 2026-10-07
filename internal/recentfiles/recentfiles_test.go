package recentfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRecentFilesRoundTrip pins the persistence contract: Add moves
// the newest entry to the front, the list stays bounded, and a fresh
// manager over the same file serves the same entries newest first.
func TestRecentFilesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recently-used.xbel")
	m := New(path)
	for i := range 5 {
		if err := m.Add(filepath.Join("/tmp", string(rune('a'+i))+".txt"), "gelm"); err != nil {
			t.Fatal(err)
		}
	}
	list := m.List()
	if len(list) != 5 || list[0].Path != "/tmp/e.txt" {
		t.Fatalf("in-memory list = %v, want newest first", list)
	}

	reloaded := New(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.List()
	if len(got) != 5 {
		t.Fatalf("reloaded list has %d entries, want 5", len(got))
	}
	if got[0].Path != "/tmp/e.txt" || got[4].Path != "/tmp/a.txt" {
		t.Errorf("reload order = %v, want newest first", got)
	}
	if got[0].App != "gelm" {
		t.Errorf("app attribution lost: %q", got[0].App)
	}
}

// TestRecentFilesReAddMovesToFront pins the dedup: re-adding an
// existing path moves it, it does not duplicate.
func TestRecentFilesReAddMovesToFront(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "recently-used.xbel"))
	_ = m.Add("/tmp/one", "gelm")
	_ = m.Add("/tmp/two", "gelm")
	_ = m.Add("/tmp/one", "gelm")
	list := m.List()
	if len(list) != 2 || list[0].Path != "/tmp/one" {
		t.Fatalf("list = %v, want one first without duplicates", list)
	}
}

// TestRecentFilesBounded pins the cap: an old entry falls off the end
// when the list outgrows maxEntries.
func TestRecentFilesBounded(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "recently-used.xbel"))
	for i := range maxEntries + 5 {
		_ = m.Add(fmt.Sprintf("/tmp/file-%03d", i), "gelm")
	}
	_ = m.Add("/tmp/newest", "gelm")
	if got := len(m.List()); got != maxEntries {
		t.Errorf("list grew to %d, want the bound %d", got, maxEntries)
	}
	if m.List()[0].Path != "/tmp/newest" {
		t.Errorf("newest = %q", m.List()[0].Path)
	}
}

// TestRecentFilesCorruptAndMissing covers the fallbacks: a missing
// file is an empty list, a corrupt one is an empty list plus the
// error, and the next Add rewrites it clean.
func TestRecentFilesCorruptAndMissing(t *testing.T) {
	dir := t.TempDir()
	missing := New(filepath.Join(dir, "none", "recently-used.xbel"))
	if err := missing.Load(); err != nil {
		t.Errorf("missing file: %v", err)
	}
	if len(missing.List()) != 0 {
		t.Error("missing file yielded entries")
	}

	path := filepath.Join(dir, "recently-used.xbel")
	if err := os.WriteFile(path, []byte("<xbel version=\"1.0\"><bookmark"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt := New(path)
	if err := corrupt.Load(); err == nil {
		t.Error("corrupt file loaded without an error")
	}
	if len(corrupt.List()) != 0 {
		t.Error("corrupt file yielded entries")
	}
	if err := corrupt.Add("/tmp/x", "gelm"); err != nil {
		t.Fatal(err)
	}
	if err := New(path).Load(); err != nil {
		t.Errorf("rewrite did not repair the file: %v", err)
	}
}

// TestRecentFilesSkipsRemoteURIs pins that only local file URIs count:
// remote hosts and other schemes drop out on load.
func TestRecentFilesSkipsRemoteURIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recently-used.xbel")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<xbel version="1.0">
  <bookmark href="file:///tmp/local.txt" modified="2026-10-07T10:00:00Z"><info><metadata owner="gelm"/></info></bookmark>
  <bookmark href="sftp://host/remote.txt" modified="2026-10-07T11:00:00Z"><info><metadata owner="gelm"/></info></bookmark>
  <bookmark href="file://otherhost/escaped.txt" modified="2026-10-07T12:00:00Z"><info><metadata owner="gelm"/></info></bookmark>
</xbel>`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(path)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	list := m.List()
	if len(list) != 1 || list[0].Path != "/tmp/local.txt" {
		t.Errorf("list = %v, want only the local file", list)
	}
	if list[0].When.IsZero() {
		t.Error("the modified timestamp was lost")
	}
}

// TestParseURIList pins the clipboard/drag uri-list consumption: CRLF
// and LF both split, comments skip, non-file URIs drop, percent
// escapes decode.
func TestParseURIList(t *testing.T) {
	got := ParseURIList("# comment\r\nfile:///tmp/a%20b.txt\r\nfile:///tmp/c.txt\nhttps://example.com/x\r\n\r\n")
	want := []string{"/tmp/a b.txt", "/tmp/c.txt"}
	if len(got) != len(want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %v, want %v", got, want)
		}
	}
}

// TestSaveEmptyListWritesEmptyXbel pins that clearing writes an empty
// document rather than deleting the shared file.
func TestSaveEmptyListWritesEmptyXbel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recently-used.xbel")
	m := New(path)
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<xbel") {
		t.Errorf("empty save wrote %q", data)
	}
}

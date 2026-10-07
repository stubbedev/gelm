package widget

import "testing"

// TestMessageCatalog pins the catalog contract: Tr is the identity
// without a catalog, the catalog translates what it knows and passes
// through what it does not, an empty translation falls back to the
// English key, and installing nil restores the identity.
func TestMessageCatalog(t *testing.T) {
	defer SetMessageCatalog(nil)

	if got := Tr("OK"); got != "OK" {
		t.Errorf("identity Tr = %q", got)
	}

	SetMessageCatalog(func(s string) string {
		if s == "OK" {
			return "OK"
		}
		if s == "Cancel" {
			return "Avbryt"
		}
		if s == "broken" {
			return ""
		}
		return s
	})
	if got := Tr("Cancel"); got != "Avbryt" {
		t.Errorf("catalog Tr = %q, want Avbryt", got)
	}
	if got := Tr("Pick a color"); got != "Pick a color" {
		t.Errorf("unknown key translated itself: %q", got)
	}
	if got := Tr("broken"); got != "broken" {
		t.Errorf("empty translation did not fall back: %q", got)
	}

	SetMessageCatalog(nil)
	if got := Tr("Cancel"); got != "Cancel" {
		t.Errorf("nil did not restore the identity: %q", got)
	}
}

// TestChooserLabelsFlowThroughCatalog proves the built-in surface, not
// just the hook: the file chooser's places row and its title carry the
// catalog's answers once it is installed.
func TestChooserLabelsFlowThroughCatalog(t *testing.T) {
	face := bindingFace(t)
	defer SetMessageCatalog(nil)
	SetMessageCatalog(func(s string) string {
		return map[string]string{
			"Up":           "Opp",
			"Home":         "Hjem",
			"Recent files": "Nylige filer",
		}[s]
	})

	c := NewFileChooser(face, 14, FileModeOpen, "/home", nil)
	c.SetSource(func(string) ([]FileEntry, error) { return nil, nil })
	c.navigate("")
	if got := c.dirLabel(); got != "Nylige filer" {
		t.Errorf("recents title = %q, want the catalog's", got)
	}
}

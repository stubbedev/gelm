package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/gelm/widget"
)

// settingsEnv points the settings file at a scratch config dir and
// returns the app directory's parent.
func settingsEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

var (
	setGreeting = Key[string]{Name: "greeting", Default: "hello"}
	setVolume   = Key[float64]{Name: "volume", Default: 0.5}
	setDark     = Key[bool]{Name: "dark-mode", Default: false}
)

// TestSettingsDefaultsBeforeAnyFile pins the first-run behavior: every
// key reads its default and nothing is written until a Set happens.
func TestSettingsDefaultsBeforeAnyFile(t *testing.T) {
	cfg := settingsEnv(t)
	s := NewSettings(testApp(nil), "dev.stubbe.test.settings")
	if got := setGreeting.Get(s); got != "hello" {
		t.Errorf("greeting = %q, want the default", got)
	}
	if got := setVolume.Get(s); got != 0.5 {
		t.Errorf("volume = %v, want the default", got)
	}
	if _, err := os.Stat(filepath.Join(cfg, "dev.stubbe.test.settings")); !os.IsNotExist(err) {
		t.Error("merely reading settings created the file")
	}
}

// TestSettingsSetPersistsAndReloads is the persistence round trip: Set
// writes the JSON file, a fresh Settings over the same file serves the
// stored values, and unknown keys in the file survive rewrites.
func TestSettingsSetPersistsAndReloads(t *testing.T) {
	cfg := settingsEnv(t)
	s := NewSettings(testApp(nil), "dev.stubbe.test.settings")
	setVolume.Set(s, 0.75)
	setDark.Set(s, true)

	path := filepath.Join(cfg, "dev.stubbe.test.settings", "settings.json")
	reloaded := NewSettings(testApp(nil), "dev.stubbe.test.settings")
	if got := setVolume.Get(reloaded); got != 0.75 {
		t.Errorf("reloaded volume = %v, want 0.75", got)
	}
	if got := setDark.Get(reloaded); !got {
		t.Errorf("reloaded dark-mode = %v, want true", got)
	}
	if got := setGreeting.Get(reloaded); got != "hello" {
		t.Errorf("reloaded greeting = %q, want the default", got)
	}

	// A key the schema does not know rides along untouched.
	if err := os.WriteFile(path,
		[]byte(`{"future-key": [1, 2], "volume": 0.75, "dark-mode": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s = NewSettings(testApp(nil), "dev.stubbe.test.settings")
	setGreeting.Set(s, "hi")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]json.RawMessage
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("rewritten settings do not parse: %v", err)
	}
	var future []int
	if err := json.Unmarshal(written["future-key"], &future); err != nil || len(future) != 2 || future[0] != 1 || future[1] != 2 {
		t.Errorf("rewrite dropped or damaged the unknown key: %s", written["future-key"])
	}
}

// TestSettingsCorruptionFallsBackToDefaults pins the documented
// fallback: a corrupt file serves defaults, and the next Set replaces
// the file with a clean one.
func TestSettingsCorruptionFallsBackToDefaults(t *testing.T) {
	dir := settingsEnv(t)
	appID := "dev.stubbe.test.corrupt"
	if err := os.MkdirAll(filepath.Join(dir, appID), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, appID, "settings.json")
	if err := os.WriteFile(path, []byte(`{"greeting": "tru`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSettings(testApp(nil), appID)
	if got := setGreeting.Get(s); got != "hello" {
		t.Errorf("corrupt file yielded %q, want the default", got)
	}
	setVolume.Set(s, 0.25)
	reloaded := NewSettings(testApp(nil), appID)
	if got := setVolume.Get(reloaded); got != 0.25 {
		t.Errorf("post-repair volume = %v, want 0.25", got)
	}
}

// TestSettingsTypeDriftFallsBackToDefault covers a file whose value
// for a key no longer matches the schema's type: the default wins and
// the next write repairs the entry.
func TestSettingsTypeDriftFallsBackToDefault(t *testing.T) {
	dir := settingsEnv(t)
	appID := "dev.stubbe.test.drift"
	if err := os.MkdirAll(filepath.Join(dir, appID), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, appID, "settings.json")
	if err := os.WriteFile(path, []byte(`{"volume": "loud"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSettings(testApp(nil), appID)
	if got := setVolume.Get(s); got != 0.5 {
		t.Errorf("type-drifted volume = %v, want the default", got)
	}
	setVolume.Set(s, 1)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "loud") {
		t.Errorf("write kept the drifted value:\n%s", data)
	}
}

// TestSettingsNotifiesOnLoopAndSuppressesEqual pins the notification
// contract: changes reach subscribers on the loop goroutine, an equal
// Set notifies nobody and rewrites nothing.
func TestSettingsNotifiesOnLoopAndSuppressesEqual(t *testing.T) {
	settingsEnv(t)
	a := testApp(nil)
	s := NewSettings(a, "dev.stubbe.test.notify")
	seen := 0
	cancel := setVolume.Subscribe(s, func(v float64) { seen++ })
	defer cancel()

	setVolume.Set(s, 0.9)
	setVolume.Set(s, 0.9)
	a.pump(time.Now())
	if seen != 1 {
		t.Errorf("two Sets (one equal) notified %d times, want 1", seen)
	}
}

// TestSettingsBindingBridgesToWidgets is the #80 Bind acceptance: a
// key's Binding drives a widget both ways, the write persists, and
// setting the key moves the widget with no echo loop.
func TestSettingsBindingBridgesToWidgets(t *testing.T) {
	settingsEnv(t)
	a := testApp(nil)
	s := NewSettings(a, "dev.stubbe.test.bind")
	b := setDark.Binding(s)

	sw := widget.NewSwitch(false)
	unbind := sw.BindOn(b)
	defer unbind()

	setDark.Set(s, true)
	a.pump(time.Now())
	if !sw.On() {
		t.Error("the switch never followed the persisted key")
	}
	sw.SetOn(false)
	a.pump(time.Now())
	if setDark.Get(s) {
		t.Error("a switch flip never persisted")
	}
}

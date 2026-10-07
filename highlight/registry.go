package highlight

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/stubbedev/gelm/widget"
)

// Language is a registered highlighter (a GtkSourceView language): its
// id, the other names it answers to (a Markdown fence's hint such as
// "golang" or "yml"), and the file-name patterns it claims.
type Language struct {
	// Name is the language id, lower-case: "go", "json".
	Name string
	// Aliases are further names Lookup accepts.
	Aliases []string
	// Globs are filepath.Match patterns against a file's base name:
	// "*.go", "Cargo.lock".
	Globs []string
	// Highlighter styles the language.
	Highlighter widget.Highlighter
}

// registry is the language table; a language's index (plus one) is
// its id inside another language's line state (a fenced block).
var registry struct {
	sync.RWMutex
	langs []Language
}

// Register adds lang, or replaces the language of the same name, so an
// application contributes its own languages (or overrides a built-in)
// without forking.
func Register(lang Language) {
	lang.Name = strings.ToLower(lang.Name)
	registry.Lock()
	defer registry.Unlock()
	for i, l := range registry.langs {
		if l.Name == lang.Name {
			registry.langs[i] = lang
			return
		}
	}
	registry.langs = append(registry.langs, lang)
}

// Lookup finds a language's highlighter by name or alias, ignoring
// case.
func Lookup(name string) (widget.Highlighter, bool) {
	if id := languageID(name); id > 0 {
		return languageAt(id), true
	}
	return nil, false
}

// ForFile finds the highlighter whose globs match path's base name.
func ForFile(path string) (widget.Highlighter, bool) {
	base := filepath.Base(path)
	registry.RLock()
	defer registry.RUnlock()
	for _, l := range registry.langs {
		for _, g := range l.Globs {
			if ok, _ := filepath.Match(g, base); ok {
				return l.Highlighter, true
			}
		}
	}
	return nil, false
}

// Languages lists the registered language names, sorted.
func Languages() []string {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]string, len(registry.langs))
	for i, l := range registry.langs {
		out[i] = l.Name
	}
	slices.Sort(out)
	return out
}

// languageID is name's registry id (index plus one), 0 when unknown.
func languageID(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return 0
	}
	registry.RLock()
	defer registry.RUnlock()
	for i, l := range registry.langs {
		if l.Name == name || slices.Contains(l.Aliases, name) {
			return i + 1
		}
	}
	return 0
}

// languageAt is the highlighter of registry id, nil when out of range.
func languageAt(id int) widget.Highlighter {
	registry.RLock()
	defer registry.RUnlock()
	if id < 1 || id > len(registry.langs) {
		return nil
	}
	return registry.langs[id-1].Highlighter
}

// builtinLanguages are the languages this package registers.
var builtinLanguages = []Language{
	{Name: "toml", Globs: []string{"*.toml", "Cargo.lock", "Pipfile"}, Highlighter: TOML{}},
	{Name: "go", Aliases: []string{"golang"}, Globs: []string{"*.go"}, Highlighter: Go{}},
	{Name: "json", Aliases: []string{"jsonc", "geojson"}, Globs: []string{"*.json", "*.geojson", "*.jsonc"}, Highlighter: JSON{}},
	{Name: "yaml", Aliases: []string{"yml"}, Globs: []string{"*.yaml", "*.yml"}, Highlighter: YAML{}},
	{Name: "markdown", Aliases: []string{"md"}, Globs: []string{"*.md", "*.markdown"}, Highlighter: Markdown{}},
}

func init() {
	for _, l := range builtinLanguages {
		Register(l)
	}
}

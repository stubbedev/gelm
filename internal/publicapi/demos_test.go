// Package publicapi pins that the user-facing demos build on the public
// API alone, so every gap in it shows up as a failing demo instead of a
// quiet internal import.
package publicapi

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/stubbedev/gelm/"

var rawProtocolTools = map[string]bool{"wlpointer": true, "zz-vpclick": true}

func TestDemosImportOnlyPublicPackages(t *testing.T) {
	cmd := filepath.Join("..", "..", "cmd")
	demos, err := os.ReadDir(cmd)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, d := range demos {
		if !d.IsDir() || rawProtocolTools[d.Name()] {
			continue
		}
		files, err := filepath.Glob(filepath.Join(cmd, d.Name(), "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range parsed.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				rest, ok := strings.CutPrefix(path, module)
				if ok && (strings.HasPrefix(rest, "internal/") || strings.HasPrefix(rest, "third_party/")) {
					t.Errorf("%s imports %s: demos use the public API only", f, path)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no demo sources found")
	}
}

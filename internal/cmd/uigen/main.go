// Command uigen writes package ui's generated builders: go generate in
// ui runs it.
package main

import (
	"log"
	"os"

	"github.com/stubbedev/gelm/internal/uigen"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: uigen <widget dir> <output file>")
	}
	src, err := uigen.Generate(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	tmp := os.Args[2] + ".tmp"
	if err := os.WriteFile(tmp, src, 0o644); err != nil { //nolint:gosec // generated source is world-readable like every other file in the tree
		log.Fatal(err)
	}
	if err := os.Rename(tmp, os.Args[2]); err != nil { //nolint:gosec // the output path is go generate's own argument
		log.Fatal(err)
	}
}

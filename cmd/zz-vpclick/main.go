// The harness clicker: pointer clicks, wheel, keys, and text on
// $WAYLAND_DISPLAY through the headless suite's virtual pointer — the
// manual companion to internal/headlesstest's in-process driving,
// used to walk a live surface (the settings app on a nested sway)
// while its trace log runs.
package main

import (
	"log"
	"os"
	"strconv"

	"github.com/stubbedev/gelm/internal/headlesstest"
)

func main() {
	x, _ := strconv.Atoi(os.Args[1])
	y, _ := strconv.Atoi(os.Args[2])
	in, err := headlesstest.Dial(os.Getenv("WAYLAND_DISPLAY"))
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 3 && os.Args[3] == "scroll" {
		dy, _ := strconv.ParseFloat(os.Args[4], 64)
		if err := in.ScrollAt(x, y, dy); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 3 && os.Args[3] == "key" {
		code, _ := strconv.Atoi(os.Args[4])
		if err := in.Tap(uint32(code)); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 3 && os.Args[3] == "type" {
		if err := in.TypeText(os.Args[4]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 3 && os.Args[3] == "dclick" {
		for range 2 {
			if err := in.ClickAt(x, y, headlesstest.BTNLeft); err != nil {
				log.Fatal(err)
			}
		}
		return
	}
	if err := in.ClickAt(x, y, headlesstest.BTNLeft); err != nil {
		log.Fatal(err)
	}
}

package headlesstest

import "fmt"

// KeyFor maps a rune to the evdev key code that produces it under the
// US layout the test keymap carries, plus whether shift must be held.
// The table covers exactly what the suite types; an unmapped rune is a
// harness bug and errors instead of silently typing the wrong key.
func KeyFor(r rune) (code uint32, shift bool, err error) {
	if c, ok := runeKeys[r]; ok {
		return c, false, nil
	}
	if c, ok := shiftedKeys[r]; ok {
		return c, true, nil
	}
	return 0, false, fmt.Errorf("headlesstest: no key code for %q; extend the table in keymap.go", r)
}

// runeKeys holds the unshifted keys the suite uses.
var runeKeys = map[rune]uint32{
	' ':  KeySpace,
	'1':  2,
	'2':  3,
	'3':  4,
	'4':  5,
	'5':  6,
	'6':  7,
	'7':  8,
	'8':  9,
	'9':  10,
	'0':  11,
	'-':  12,
	'=':  13,
	'\t': KeyTab,
	'q':  16,
	'w':  17,
	'e':  18,
	'r':  19,
	't':  20,
	'y':  21,
	'u':  22,
	'i':  23,
	'o':  24,
	'p':  25,
	'[':  26,
	']':  27,
	'a':  KeyA,
	's':  31,
	'd':  32,
	'f':  33,
	'g':  34,
	'h':  35,
	'j':  36,
	'k':  37,
	'l':  38,
	';':  39,
	'\'': 40,
	'z':  44,
	'x':  45,
	'c':  KeyC,
	'v':  KeyV,
	'b':  48,
	'n':  49,
	'm':  50,
	',':  51,
	'.':  52,
	'/':  53,
}

// shiftedKeys holds the shifted keys the suite uses.
var shiftedKeys = map[rune]uint32{
	'!': 2,
	'@': 3,
	'#': 4,
	'$': 5,
	'%': 6,
	'^': 7,
	'&': 8,
	'*': 9,
	'(': 10,
	')': 11,
	'A': KeyA,
	'B': 48,
	'C': KeyC,
	'D': 32,
	'E': 18,
	'F': 33,
	'G': 34,
	'H': 35,
	'I': 23,
	'J': 36,
	'K': 37,
	'L': 38,
	'M': 50,
	'N': 49,
	'O': 24,
	'P': 25,
	'Q': 16,
	'R': 19,
	'S': 31,
	'T': 20,
	'U': 22,
	'V': KeyV,
	'W': 17,
	'X': 45,
	'Y': 21,
	'Z': 44,
}

package headlesstest

import (
	"fmt"
	"strings"
	"testing"
)

func pressSerial(tr Trace) (uint32, bool) {
	if tr.Category != "input" {
		return 0, false
	}
	var code, state, serial uint32
	if _, err := fmt.Sscanf(tr.Message, "wire button %d state=%d serial=%d", &code, &state, &serial); err == nil {
		return serial, state == 1
	}
	if _, err := fmt.Sscanf(tr.Message, "wire key code=%d state=%d serial=%d", &code, &state, &serial); err == nil {
		return serial, state == 1
	}
	return 0, false
}

func grabSerialMismatches(traces []Trace) (grabs int, mismatches []string) {
	var last uint32
	for _, tr := range traces {
		if serial, ok := pressSerial(tr); ok {
			last = serial
			continue
		}
		if tr.Category != "input" || !strings.HasPrefix(tr.Message, "popup grab serial=") {
			continue
		}
		var grab uint32
		if _, err := fmt.Sscanf(tr.Message, "popup grab serial=%d", &grab); err != nil {
			mismatches = append(mismatches, fmt.Sprintf("unparsable %q", tr.Message))
			continue
		}
		grabs++
		if grab == 0 || grab != last {
			mismatches = append(mismatches, fmt.Sprintf("grab serial %d after the press serial %d", grab, last))
		}
	}
	return grabs, mismatches
}

func requireGrabsCarryTheTriggeringSerial(t *testing.T, w *LogWatcher, mark, want int) {
	t.Helper()
	grabs, mismatches := grabSerialMismatches(w.Since(mark))
	if grabs < want {
		t.Errorf("saw %d popup grabs, want at least %d\n%s", grabs, want, tailTraces(w, 25))
	}
	for _, m := range mismatches {
		t.Errorf("a popup grab does not carry the serial of the press that opened it: %s", m)
	}
}

func TestGrabSerialMatching(t *testing.T) {
	traces := ParseTraces([]byte("" +
		"  0.100s input: wire button 272 state=1 serial=9\n" +
		"  0.110s input: wire button 272 state=0 serial=10\n" +
		"  0.120s input: popup grab serial=9\n" +
		"  0.200s input: wire key code=106 state=1 serial=11 mods=0x0\n" +
		"  0.210s input: popup grab serial=11\n" +
		"  0.300s input: wire key code=106 state=0 serial=12 mods=0x0\n" +
		"  0.310s input: popup grab serial=0\n"))
	grabs, mismatches := grabSerialMismatches(traces)
	if grabs != 3 || len(mismatches) != 1 || !strings.Contains(mismatches[0], "grab serial 0 after the press serial 11") {
		t.Errorf("grabs %d, mismatches %v; want 3 grabs and only the zero grab flagged", grabs, mismatches)
	}
}

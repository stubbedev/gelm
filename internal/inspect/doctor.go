// The doctor diagnostics block: one line block a bug report carries
// verbatim ("here is my gelm doctor output"), covering the session's
// bound globals and their versions, outputs and scales, the resolved
// cursor theme, the fonts sysfont selected, and the shm buffer
// format. Everything is read-only snapshot data; the app prints it at
// startup behind a flag, and nothing here requires the inspector to
// be armed.
package inspect

import (
	"maps"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/wlsession"
)

// DoctorOptions carries the doctor's overridable inputs. Empty fields
// mean "resolve it yourself": the font names come from sysfont's
// default resolution.
type DoctorOptions struct {
	// Sans and Mono name the resolved default faces; empty resolves
	// them through sysfont.Sans/Monospace at format time.
	Sans, Mono string
}

// Doctor renders the diagnostics block from a live session. A nil
// session formats the non-session lines (fonts, buffer) and marks the
// session lines unavailable, so a doctor run before Connect still
// produces the block.
func Doctor(sess *wlsession.Session, opts DoctorOptions) string {
	info := wlsession.InspectInfo{}
	if sess != nil {
		info = sess.Inspect()
	}
	return DoctorFrom(info, opts)
}

// DoctorFrom renders the diagnostics block from a session snapshot.
// The format is stable: tests pin every line's shape, so tooling (and
// humans skimming an issue) can rely on it.
func DoctorFrom(info wlsession.InspectInfo, opts DoctorOptions) string {
	var b strings.Builder
	b.WriteString("gelm doctor\n")
	b.WriteString("go: " + runtime.Version() + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")\n")

	switch {
	case info.Globals == nil:
		b.WriteString("globals: (no session)\n")
	case len(info.Globals) == 0:
		b.WriteString("globals: (none bound)\n")
	default:
		parts := make([]string, 0, len(info.Globals))
		for _, g := range slices.Sorted(maps.Keys(info.Globals)) {
			parts = append(parts, g+" v"+strconv.FormatUint(uint64(info.Globals[g]), 10))
		}
		b.WriteString("globals: " + strings.Join(parts, ", ") + "\n")
	}

	if len(info.Outputs) == 0 {
		b.WriteString("outputs: (none)\n")
	}
	for _, o := range info.Outputs {
		b.WriteString("outputs: " + describeOutput(o) + "\n")
	}

	switch {
	case info.CursorErr != nil:
		b.WriteString("cursor theme: unavailable (" + info.CursorErr.Error() + ")\n")
	case info.CursorTheme == "":
		b.WriteString("cursor theme: unavailable (not resolved)\n")
	default:
		b.WriteString("cursor theme: " + info.CursorTheme + " " +
			strconv.Itoa(info.CursorSize) + "px\n")
	}

	b.WriteString("fonts: " + describeFonts(opts) + "\n")
	b.WriteString("buffer: ")
	b.WriteString(buffer.FormatName)
	b.WriteByte('\n')
	if info.FractionalScale {
		b.WriteString("fractional scale: yes (wp_viewporter + wp_fractional_scale_v1)\n")
	} else {
		b.WriteString("fractional scale: no (integer set_buffer_scale)\n")
	}
	return b.String()
}

// describeOutput renders one output line: stable name when the
// compositor provides xdg_output, then mode, integer scale, and the
// logical geometry the fractional scale applies to.
func describeOutput(o *wlsession.Output) string {
	name := o.Name
	if name == "" {
		name = "(unnamed)"
	}
	return name + " mode " + strconv.Itoa(o.ModeW) + "x" + strconv.Itoa(o.ModeH) +
		" scale " + strconv.Itoa(o.Scale) +
		" logical " + strconv.Itoa(int(o.LogicalX)) + "," + strconv.Itoa(int(o.LogicalY)) + "+" +
		strconv.Itoa(int(o.LogicalW)) + "x" + strconv.Itoa(int(o.LogicalH))
}

// describeFonts renders the font line: the resolved default faces and
// the installed fallback families, so a missing-face bug report shows
// what the selector actually picked.
func describeFonts(opts DoctorOptions) string {
	sans, mono := opts.Sans, opts.Mono
	if sans == "" {
		if tf, err := sysfont.Sans(); err == nil {
			sans = tf.Family()
		} else {
			sans = "<unresolved: " + err.Error() + ">"
		}
	}
	if mono == "" {
		if tf, err := sysfont.Monospace(); err == nil {
			mono = tf.Family()
		} else {
			mono = "<unresolved: " + err.Error() + ">"
		}
	}
	line := "sans=" + strconv.Quote(sans) + " mono=" + strconv.Quote(mono)
	if fams := sysfont.FallbackFamilies(); len(fams) > 0 {
		line += " fallback=[" + strings.Join(fams, ", ") + "]"
	}
	return line
}

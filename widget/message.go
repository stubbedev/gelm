package widget

import "sync"

// The message catalog: every built-in string the toolkit paints flows
// through Tr, so an application localizes gelm's chrome - dialog
// buttons, chooser labels, places, status lines - with one hook
// (#89). The default catalog is the identity: untranslated gelm is
// exactly today's English.
//
// The lookup key is the English string itself (the gettext msgid
// model), so a catalog is a plain map:
//
//	widget.SetMessageCatalog(func(s string) string {
//		return map[string]string{
//			"OK":     "OK",
//			"Cancel": "Avbryt",
//		}[s]
//	})
//
// Locale detection is the application's: read LANG/LC_MESSAGES (or
// the portal's org.freedesktop.appearance settings) and load your own
// catalog - gelm deliberately ships no catalog format, no plural
// rules, and no locale parsing; apps bring their own loader (golang.
// org/x/text/message, gotext, a map literal - all fit the hook).
//
// The strings that flow through Tr are user-facing chrome. Developer
// surfaces - the inspector's dump, trace log lines - stay English on
// purpose: they are read by the person debugging, not the person
// using. Install the catalog before building widgets; it is a
// process-wide setting like SetTheme, safe from any goroutine.

var catalog struct {
	mu sync.RWMutex
	tr func(string) string
}

// SetMessageCatalog installs lookup as the translation for every
// built-in string; nil restores the identity. Call once at startup,
// before widgets are built.
func SetMessageCatalog(lookup func(string) string) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	catalog.tr = lookup
}

// Tr translates one built-in string through the installed catalog:
// the hook point every toolkit-owned label passes through. English in,
// catalog's answer out, English when no catalog is installed.
func Tr(s string) string {
	catalog.mu.RLock()
	tr := catalog.tr
	catalog.mu.RUnlock()
	if tr == nil {
		return s
	}
	if out := tr(s); out != "" {
		return out
	}
	return s
}

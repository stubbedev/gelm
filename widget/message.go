package widget

import "sync"

// Catalog translates user-facing strings, the gettext model: the key
// is the English string itself, plural pairs choose a form for n, and
// a context disambiguates equal English strings. *i18n.Catalog
// implements it from .po and .mo files; an untranslated key comes back
// unchanged.
type Catalog interface {
	Get(id string) string
	GetN(singular, plural string, n int) string
	GetCtx(ctx, id string) string
}

var catalog struct {
	mu sync.RWMutex
	c  Catalog
}

// SetMessageCatalog installs c as the translation for every built-in
// string and every Tr, TrN and TrCtx call; nil restores the identity.
// It is process-wide like SetTheme: install it before building widgets.
func SetMessageCatalog(c Catalog) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	catalog.c = c
}

func installedCatalog() Catalog {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	return catalog.c
}

// Tr translates s through the installed catalog, English without one.
// Every toolkit-owned label passes through it; developer surfaces (the
// inspector dump, traces) stay English on purpose.
func Tr(s string) string {
	c := installedCatalog()
	if c == nil {
		return s
	}
	return orEnglish(c.Get(s), s)
}

// TrN translates a plural pair for n: singular for 1 and plural
// otherwise without a catalog, the catalog's form for n with one.
func TrN(singular, plural string, n int) string {
	english := plural
	if n == 1 {
		english = singular
	}
	c := installedCatalog()
	if c == nil {
		return english
	}
	return orEnglish(c.GetN(singular, plural, n), english)
}

// TrCtx translates s within ctx (gettext's msgctxt).
func TrCtx(ctx, s string) string {
	c := installedCatalog()
	if c == nil {
		return s
	}
	return orEnglish(c.GetCtx(ctx, s), s)
}

func orEnglish(translated, english string) string {
	if translated == "" {
		return english
	}
	return translated
}

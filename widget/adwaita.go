package widget

import (
	"fmt"
	"strings"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget/css"
)

// The theme layer (#95): a StylePriorityTheme stylesheet generated
// from the current Theme that defines libadwaita's named colors (as
// @define-color, so @accent_bg_color and friends resolve in any
// stylesheet) and styles the css package's classes in terms of them.
// SetTheme regenerates it, so a palette switch re-derives every named
// color; an application stylesheet outranks every rule here.

// themeSheet is the installed theme layer (installed at init).
var themeSheet *Stylesheet

// syncThemeSheet (re)generates the theme layer from t.
func syncThemeSheet(t *Theme) {
	src := themeCSS(t)
	if themeSheet == nil {
		themeSheet = AddStylesheet(src, StylePriorityTheme)
		return
	}
	themeSheet.Load(src)
}

// namedColors maps each Adwaita named color to its value under t.
func namedColors(t *Theme) [][2]string {
	white := render.RGB(0xff, 0xff, 0xff)
	shade := t.ShadowColor
	if shade == 0 {
		shade = render.RGBA(0, 0, 0, 0x12)
	}
	pairs := []struct {
		name string
		c    Color
	}{
		{css.AccentBgColor, t.Accent},
		{css.AccentFgColor, t.OnAccent},
		{css.AccentColor, t.Accent},
		{css.DestructiveBgColor, t.DestructiveColor()},
		{css.DestructiveFgColor, white},
		{css.DestructiveColor, t.DestructiveColor()},
		{css.SuccessBgColor, t.SuccessColor()},
		{css.SuccessFgColor, white},
		{css.SuccessColor, t.SuccessColor()},
		{css.WarningBgColor, t.WarningColor()},
		{css.WarningFgColor, render.RGBA(0, 0, 0, 0xcc)},
		{css.WarningColor, t.WarningColor()},
		{css.ErrorBgColor, t.ErrorColor()},
		{css.ErrorFgColor, white},
		{css.ErrorColor, t.ErrorColor()},
		{css.WindowBgColor, t.Bg},
		{css.WindowFgColor, t.Text},
		{css.ViewBgColor, t.Surface},
		{css.ViewFgColor, t.Text},
		{css.HeaderbarBgColor, t.Surface},
		{css.HeaderbarFgColor, t.Text},
		{css.HeaderbarBorderColor, t.Border},
		{css.CardBgColor, t.Surface},
		{css.CardFgColor, t.Text},
		{css.DialogBgColor, t.Bg},
		{css.DialogFgColor, t.Text},
		{css.PopoverBgColor, t.Surface},
		{css.PopoverFgColor, t.Text},
		{css.SidebarBgColor, t.Surface},
		{css.SidebarFgColor, t.Text},
		{css.ShadeColor, shade},
		{css.BordersColor, t.Border},
	}
	out := make([][2]string, len(pairs))
	for i, p := range pairs {
		out[i] = [2]string{p.name, FormatColor(p.c)}
	}
	return out
}

// classRules styles the css package's classes in terms of the named
// colors; Activatable and Numeric are behavior and face hints with no
// paint of their own.
var classRules = [][2]string{
	{css.Accent, "color: @accent_color;"},
	{css.Success, "color: @success_color;"},
	{css.Warning, "color: @warning_color;"},
	{css.Error, "color: @error_color;"},
	{css.DimLabel, "opacity: 0.55;"},
	{css.Heading, "font-weight: 700;"},
	{css.Caption, "font-size: 12px;"},
	{css.CaptionHeading, "font-size: 12px; font-weight: 700;"},
	{css.Title1, "font-size: 28px; font-weight: 800;"},
	{css.Title2, "font-size: 22px; font-weight: 800;"},
	{css.Title3, "font-size: 20px; font-weight: 700;"},
	{css.Title4, "font-size: 17px; font-weight: 700;"},
	{css.Monospace, "font-family: monospace;"},
	{css.Flat, "background-color: transparent;"},
	{css.SuggestedAction, "background-color: @accent_bg_color; color: @accent_fg_color;"},
	{css.DestructiveAction, "background-color: @destructive_bg_color; color: @destructive_fg_color;"},
	{css.Pill, "border-radius: 9999px; padding: 10px 32px;"},
	{css.Circular, "border-radius: 9999px;"},
	{css.Card, "background-color: @card_bg_color; color: @card_fg_color; border-radius: 12px;"},
	{css.BoxedList, "background-color: @card_bg_color; border-radius: 12px;"},
	{css.OSD, "background-color: #000000b3; color: #ffffff;"},
	{css.View, "background-color: @view_bg_color; color: @view_fg_color;"},
	{css.Toolbar, "padding: 6px;"},
	{css.Keycap, "font-size: 12px; padding: 2px 6px; border: 1px solid @borders; border-radius: 6px; background-color: @card_bg_color;"},
}

// partRules style widget parts whose default paint is a named color:
// the level bar's trough and offset classes.
var partRules = [][2]string{
	{"levelbar trough", "background-color: @view_bg_color;"},
	{"levelbar block." + LevelBarOffsetLow, "background-color: @warning_bg_color;"},
	{"levelbar block." + LevelBarOffsetHigh, "background-color: @accent_bg_color;"},
	{"levelbar block." + LevelBarOffsetFull, "background-color: @success_bg_color;"},
}

// themeCSS renders the theme layer for t.
func themeCSS(t *Theme) string {
	var b strings.Builder
	for _, nc := range namedColors(t) {
		fmt.Fprintf(&b, "@define-color %s %s;\n", nc[0], nc[1])
	}
	for _, r := range classRules {
		fmt.Fprintf(&b, ".%s { %s }\n", r[0], r[1])
	}
	for _, r := range partRules {
		fmt.Fprintf(&b, "%s { %s }\n", r[0], r[1])
	}
	return b.String()
}

// init installs the theme layer for the default palette, so the named
// colors resolve before any SetTheme.
func init() { syncThemeSheet(current) }

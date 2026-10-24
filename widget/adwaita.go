package widget

import (
	"fmt"
	"strings"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
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
	syncSystemColors(t)
	src := themeCSS(t)
	if themeSheet == nil {
		themeSheet = AddStylesheet(src, StylePriorityTheme)
		return
	}
	themeSheet.Load(src)
}

// syncSystemColors maps the CSS system color keywords onto the theme,
// the same re-derivation a palette swap gives the named colors.
func syncSystemColors(t *Theme) {
	gray := Color(0)
	if a := t.Text.Straight(); a[3] > 0 {
		gray = render.RGBA(a[0], a[1], a[2], a[3]/2)
	}
	style.SetSystemColors(style.SystemColors{
		Canvas: t.Bg, CanvasText: t.Text,
		LinkText: t.Accent, VisitedText: t.Accent, GrayText: gray,
		Highlight: t.Accent, HighlightText: t.OnAccent,
		Mark: t.Accent, MarkText: t.OnAccent,
		ButtonFace: t.Surface, ButtonText: t.Text, ButtonBorder: t.Border,
		Field: t.Surface, FieldText: t.Text,
	})
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
		{css.CardBgColor, t.CardColor()},
		{css.CardFgColor, t.Text},
		{css.DialogBgColor, t.Bg},
		{css.DialogFgColor, t.Text},
		{css.PopoverBgColor, t.PopoverColor()},
		{css.PopoverFgColor, t.Text},
		{css.SidebarBgColor, t.Surface},
		{css.SidebarFgColor, t.Text},
		{css.ShadeColor, shade},
		{css.BordersColor, t.Border},
		{css.MutedBgColor, t.MutedColor()},
		{css.MutedFgColor, t.DisabledText()},
		{css.InputBorderColor, t.InputColor()},
		{css.RingColor, t.RingColor()},
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
	{css.Card, "background-color: @card_bg_color; color: @card_fg_color; border: 1px solid @borders; border-radius: 12px; box-shadow: 0px 1px 2px alpha(black, 0.05);"},
	{css.BoxedList, "background-color: @card_bg_color; border-radius: 12px;"},
	{css.OSD, "background-color: #000000b3; color: #ffffff;"},
	{css.View, "background-color: @view_bg_color; color: @view_fg_color;"},
	{css.Toolbar, "padding: 6px;"},
	{css.Keycap, "font-size: 12px; padding: 2px 6px; border: 1px solid @borders; border-radius: 6px; background-color: @card_bg_color;"},
}

// variants are the button classes that take shadcn/ui's full button
// shape; a plain button keeps the padding and radius its constructor
// was given, so internal buttons (calendar navigation, column headers,
// window controls) keep theirs.
var variants = []string{css.SuggestedAction, css.DestructiveAction, css.Secondary, css.Outline, css.Ghost, css.Link}

// controlRules give the controls shadcn/ui's look: hairline input
// borders, the button variants and sizes, and the floating surfaces.
// Every color is a named color, so a palette switch restyles them.
func controlRules() [][2]string {
	var all strings.Builder
	for i, v := range variants {
		if i > 0 {
			all.WriteString(", ")
		}
		all.WriteString("button." + v)
	}
	return [][2]string{
		{"button", "font-weight: 500;"},
		{all.String(), "min-height: 36px; padding: 0px 16px; border-radius: 8px;"},
		{"button." + css.Small, "min-height: 32px; padding: 0px 12px; border-radius: 8px;"},
		{"button." + css.Large, "min-height: 40px; padding: 0px 32px; border-radius: 8px;"},
		{"button." + css.SuggestedAction + ":hover", "background-color: mix(@accent_bg_color, @window_bg_color, 0.1);"},
		{"button." + css.SuggestedAction + ":active", "background-color: mix(@accent_bg_color, @window_bg_color, 0.2);"},
		{"button." + css.SuggestedAction + ":disabled", "background-color: alpha(@accent_bg_color, 0.5);"},
		{"button." + css.DestructiveAction + ":hover", "background-color: mix(@destructive_bg_color, @window_bg_color, 0.1);"},
		{"button." + css.DestructiveAction + ":active", "background-color: mix(@destructive_bg_color, @window_bg_color, 0.2);"},
		{"button." + css.DestructiveAction + ":disabled", "background-color: alpha(@destructive_bg_color, 0.5);"},
		{"button." + css.Outline, "background-color: @window_bg_color; border: 1px solid @input_border_color; box-shadow: 0px 1px 2px alpha(black, 0.05);"},
		{"button." + css.Outline + ":hover, button." + css.Ghost + ":hover", "background-color: @muted_bg_color;"},
		{"button." + css.Ghost + ", button." + css.Link, "background-color: transparent;"},
		{"button." + css.Ghost + ":disabled, button." + css.Link + ":disabled, button." + css.Outline + ":disabled", "background-color: transparent;"},
		{"button." + css.Link, "color: @accent_color;"},
		{"button." + css.Link + ":hover label", "text-decoration-line: underline;"},
		{"dropdown > button", "background-color: transparent; box-shadow: inset 0px 0px 0px 1px @input_border_color; border-radius: 8px;"},
		{"dropdown > button:hover", "background-color: @muted_bg_color;"},
		{"checkbutton:checked check", "border-color: @accent_bg_color;"},
		{"scale slider", "background-color: @window_bg_color; box-shadow: inset 0px 0px 0px 1px @accent_bg_color;"},
		{"entry, spinbutton, textview", "box-shadow: inset 0px 0px 0px 1px @input_border_color; border-radius: 8px; background-color: transparent;"},
		{"entry:focus, spinbutton:focus, textview:focus", "box-shadow: inset 0px 0px 0px 1px @ring_color;"},
		{"popover, menu", "background-color: @popover_bg_color; color: @popover_fg_color; border: 1px solid @borders; border-radius: 8px;"},
	}
}

// partRules style widget parts: the level bar's trough and offset
// classes, and the media controls' volume slider length.
var partRules = [][2]string{
	{"levelbar trough", "background-color: @view_bg_color;"},
	{"levelbar block." + LevelBarOffsetLow, "background-color: @warning_bg_color;"},
	{"levelbar block." + LevelBarOffsetHigh, "background-color: @accent_bg_color;"},
	{"levelbar block." + LevelBarOffsetFull, "background-color: @success_bg_color;"},
	{"controls .volume trough", "min-width: 80px;"},
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
	for _, r := range controlRules() {
		fmt.Fprintf(&b, "%s { %s }\n", r[0], r[1])
	}
	for _, r := range partRules {
		fmt.Fprintf(&b, "%s { %s }\n", r[0], r[1])
	}
	return b.String()
}

// init installs the theme layer for the default palette, so the named
// colors resolve before any SetTheme.
func init() { syncThemeSheet(current) }

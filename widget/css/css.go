// Package css names libadwaita's style classes and named colors - the
// relm4-css analog (#95) - so application code spells them as
// constants and stylesheets written for GTK port over verbatim:
//
//	button.AddClass(css.DestructiveAction)
//	widget.LoadStylesheet(`.sidebar { background-color: @window_bg_color; }`)
//
// The widget package's theme layer styles every class listed here and
// defines every named color from the current theme (widget.SetTheme),
// at StylePriorityTheme, so an application stylesheet overrides any of
// them and a theme switch re-derives them all.
package css

// Style classes, the libadwaita names.
const (
	// Accent paints text in the accent color.
	Accent = "accent"
	// Activatable marks a row as clickable as a whole.
	Activatable = "activatable"
	// BoxedList is a list drawn as a rounded card with row separators.
	BoxedList = "boxed-list"
	// Caption is small secondary text.
	Caption = "caption"
	// CaptionHeading is small bold text.
	CaptionHeading = "caption-heading"
	// Card is a raised rounded surface.
	Card = "card"
	// Circular makes a button round.
	Circular = "circular"
	// DestructiveAction marks a button whose action loses data.
	DestructiveAction = "destructive-action"
	// DimLabel fades text to secondary emphasis.
	DimLabel = "dim-label"
	// Error paints text in the error color.
	Error = "error"
	// Flat removes a button's resting background.
	Flat = "flat"
	// Heading is bold body text.
	Heading = "heading"
	// Keycap draws a label as a key on a keyboard (a shortcut's keys).
	Keycap = "keycap"
	// Monospace sets the monospace family.
	Monospace = "monospace"
	// Numeric uses tabular figures (a hint; the face decides).
	Numeric = "numeric"
	// OSD is the dark translucent on-screen-display surface.
	OSD = "osd"
	// Pill makes a button a wide rounded pill.
	Pill = "pill"
	// Success paints text in the success color.
	Success = "success"
	// SuggestedAction marks the affirmative button.
	SuggestedAction = "suggested-action"
	// Title1 through Title4 are the display heading sizes.
	Title1 = "title-1"
	Title2 = "title-2"
	Title3 = "title-3"
	Title4 = "title-4"
	// Toolbar spaces a row of buttons as a toolbar.
	Toolbar = "toolbar"
	// View is the content-area surface.
	View = "view"
	// Warning paints text in the warning color.
	Warning = "warning"
)

// Named colors, referenced in stylesheets as @name.
const (
	AccentBgColor        = "accent_bg_color"
	AccentFgColor        = "accent_fg_color"
	AccentColor          = "accent_color"
	DestructiveBgColor   = "destructive_bg_color"
	DestructiveFgColor   = "destructive_fg_color"
	DestructiveColor     = "destructive_color"
	SuccessBgColor       = "success_bg_color"
	SuccessFgColor       = "success_fg_color"
	SuccessColor         = "success_color"
	WarningBgColor       = "warning_bg_color"
	WarningFgColor       = "warning_fg_color"
	WarningColor         = "warning_color"
	ErrorBgColor         = "error_bg_color"
	ErrorFgColor         = "error_fg_color"
	ErrorColor           = "error_color"
	WindowBgColor        = "window_bg_color"
	WindowFgColor        = "window_fg_color"
	ViewBgColor          = "view_bg_color"
	ViewFgColor          = "view_fg_color"
	HeaderbarBgColor     = "headerbar_bg_color"
	HeaderbarFgColor     = "headerbar_fg_color"
	HeaderbarBorderColor = "headerbar_border_color"
	CardBgColor          = "card_bg_color"
	CardFgColor          = "card_fg_color"
	DialogBgColor        = "dialog_bg_color"
	DialogFgColor        = "dialog_fg_color"
	PopoverBgColor       = "popover_bg_color"
	PopoverFgColor       = "popover_fg_color"
	SidebarBgColor       = "sidebar_bg_color"
	SidebarFgColor       = "sidebar_fg_color"
	ShadeColor           = "shade_color"
	BordersColor         = "borders"
)

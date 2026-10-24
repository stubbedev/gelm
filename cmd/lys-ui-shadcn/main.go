// Command lys-ui-shadcn is the shadcn/ui gallery: every shadcn
// component lys-ui implements, one titled section each, declared with
// the typed ui builders in the shadcn dark and light palettes. t or the
// header button toggles the palette, Escape closes.
package main

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

func main() {
	switch err := run(); {
	case err == nil, errors.Is(err, app.ErrClosed):
	case errors.Is(err, app.ErrDisconnected):
		os.Exit(app.DisconnectExitCode)
	default:
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.UIFont(14)
	if err != nil {
		return err
	}
	application := app.NewApplication(sess)
	var ctrl *component.Controller[galleryMsg, struct{}]
	ctrl, _, err = component.Window(application, app.WindowConfig{
		Title:      "lys-ui shadcn gallery",
		AppID:      "dev.stubbe.lys.ui.shadcn",
		Width:      760,
		Height:     820,
		Background: widget.Current().Bg,
		OnKey: func(_ *widget.Router, code uint32, _ app.Mods) {
			switch application.KeySym(code) {
			case xkb.KeyEscape:
				application.Quit()
			case xkb.Keysym('t'):
				ctrl.Send(togglePalette{})
			}
		},
	}, &gallery{env: ui.Env{Face: app.FontFallback(face), Size: 14}})
	if err != nil {
		return err
	}
	return application.Run()
}

type galleryMsg interface{ galleryMsg() }

type togglePalette struct{}

type submitted struct{ name string }

func (togglePalette) galleryMsg() {}
func (submitted) galleryMsg()     {}

type gallery struct {
	env    ui.Env
	status string
}

// form is the Field and Form section: two required entries and a
// switch, validated on blur and on submit.
func (g *gallery) form(cx *component.Context[galleryMsg, struct{}]) ui.Node {
	var name, email *widget.Entry
	var form *widget.Form
	required := func(e **widget.Entry, what string) func() string {
		return func() string {
			if strings.TrimSpace((*e).Text()) == "" {
				return what + " is required."
			}
			return ""
		}
	}
	return ui.Column(
		ui.Form().Ref(&form).
			Append(ui.Label("Profile").Classes(css.H4)).
			Add(ui.Field("Name", ui.Entry().Ref(&name)).Description("Your public display name."), required(&name, "Your name")).
			Add(ui.Field("Email", ui.Entry().Ref(&email)), func() string {
				if !strings.Contains(email.Text(), "@") {
					return "Enter a valid email address."
				}
				return ""
			}).
			Add(ui.Field("Newsletter", ui.Switch(true)).Description("Monthly product updates.").Orientation(widget.FieldHorizontal), nil).
			OnSubmit(func() { cx.Input(submitted{name: name.Text()}) }),
		ui.Row(
			ui.Button(ui.Label("Submit"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { form.Submit() }),
			ui.Label("").Classes(css.Muted).WatchText(func() string { return g.status }),
		).Spacing(16),
	).Spacing(24)
}

func (g *gallery) Init(cx *component.Context[galleryMsg, struct{}]) widget.Widget {
	section := func(title string, body ...ui.Node) ui.Node {
		return ui.Column(append([]ui.Node{ui.Label(title).Classes(css.H4)}, body...)...).Spacing(12)
	}
	text := func(s string) ui.Node { return ui.Label(s).Wrap(true) }
	header := ui.Row(
		ui.Expand(ui.Label("shadcn/ui in lys-ui").Classes(css.H2)),
		ui.Button(ui.Label("Toggle palette"), 0, 0).Classes(css.Outline).
			OnClick(func() { cx.Input(togglePalette{}) }),
	).Spacing(16)
	return ui.Mount(cx, g.env, ui.Scroll(ui.Clamp(680, ui.Column(
		header,
		section("Accordion", ui.Accordion(widget.AccordionSingle).Collapsible(true).
			Append(ui.AccordionItem("Is it accessible?", text("Yes. The triggers take focus, and Up, Down, Home and End move between them."))).
			Append(ui.AccordionItem("Is it styled?", text("Yes. It comes with default styles that match the other components' aesthetic.")).Open(true)).
			Append(ui.AccordionItem("Is it animated?", text("Yes. The content height and the chevron animate unless reduced motion is on.")))),
		section("Alert",
			ui.Alert("Heads up!", "You can add components to your app using the ui builders.").
				Icon(ui.Symbol(widget.SymbolCrosshair, 16)),
			ui.Alert("Unable to process your payment.", "Please verify your billing information and try again.").
				Icon(ui.Symbol(widget.SymbolClose, 16)).Variant(widget.AlertDestructive)),
		section("Badge", ui.Row(
			ui.Badge("Default"),
			ui.Badge("Secondary").Variant(widget.BadgeSecondary),
			ui.Badge("Destructive").Variant(widget.BadgeDestructive),
			ui.Badge("Outline").Variant(widget.BadgeOutline),
		).Spacing(8)),
		section("Breadcrumb",
			ui.Breadcrumb("Home", "Components", "Breadcrumb"),
			ui.Breadcrumb("Home", "Docs", "Components", "Data", "Breadcrumb").MaxItems(3)),
		section("Empty", ui.StatusPage("", "No projects yet", "You haven't created any projects yet. Get started by creating your first project.").
			Icon(ui.Symbol(widget.SymbolMaximize, 24)).
			Child(ui.Button(ui.Label("Create project"), 0, 0).Classes(css.SuggestedAction))),
		section("Field and Form", g.form(cx)),
		section("Input Group",
			ui.InputGroup(ui.Entry().Placeholder("Search...")).
				Prepend(ui.Symbol(widget.SymbolCrosshair, 16)).Append(ui.Label("12 results")),
			ui.InputGroup(ui.Entry().Text("example.com")).
				Prepend(ui.Label("https://")).
				Append(ui.Button(ui.Label("Copy"), 0, 0).Classes(css.Ghost, css.Small))),
		section("Kbd", ui.Row(ui.KbdGroup("Ctrl", "Shift", "P"), ui.Label("opens the command palette")).Spacing(12)),
		section("Pagination", ui.Pagination(10, 5)),
		section("Skeleton", ui.Row(
			ui.Skeleton(48, 48),
			ui.Column(ui.Skeleton(240, 14), ui.Skeleton(180, 14)).Spacing(8),
		).Spacing(16)),
		section("Typography",
			ui.Label("The Joke Tax Chronicles").Classes(css.H1),
			ui.Label("Once upon a time, in a far-off land, there was a very lazy king who spent all day lounging on his throne.").Classes(css.Lead).Wrap(true),
			ui.Label("The King's Plan").Classes(css.H2),
			ui.Label("\"After all,\" he said, \"everyone enjoys a good joke, so it's only fair that they should pay for the privilege.\"").Classes(css.Blockquote).Wrap(true),
			ui.BulletList("1st level of puns: 5 gold coins", "2nd level of jokes: 10 gold coins", "3rd level of one-liners: 20 gold coins"),
			ui.Aligned(ui.Label("ui.Label(text).Classes(css.InlineCode)").Classes(css.InlineCode), widget.AlignStart),
			ui.Label("Enter your email address.").Classes(css.Muted),
		),
	).Spacing(40).Padding(render.UniformInsets(32)))).VerticalOnly(true).Classes(css.Background))
}

func (g *gallery) Update(_ *component.Context[galleryMsg, struct{}], msg galleryMsg) {
	switch m := msg.(type) {
	case submitted:
		g.status = "Saved " + m.name + "."
	case togglePalette:
		if widget.Current().IsDark() {
			widget.SetTheme(widget.LightTheme())
		} else {
			widget.SetTheme(widget.DarkTheme())
		}
	}
}

package component_test

import (
	"errors"
	"log"
	"strconv"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

type Msg int

const (
	Increment Msg = iota
	Decrement
)

type Counter struct {
	face *render.Typeface
	n    int
}

func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	label := widget.NewLabel(c.face, 15, "", widget.Current().Text)
	cx.Watch(func() { label.SetText(strconv.Itoa(c.n)) })

	inc := widget.NewButton(widget.NewLabel(c.face, 15, "+", widget.Current().Text), 8, 6)
	inc.OnClick = func() { cx.Input(Increment) }

	box := widget.NewBox(widget.Row, 6, 6)
	box.Append(inc, false)
	box.Append(label, true)
	return box
}

func (c *Counter) Update(cx *component.Context[Msg, int], msg Msg) {
	switch msg {
	case Increment:
		c.n++
	case Decrement:
		c.n--
	}
	cx.Output(c.n)
}

func ExampleWindow() {
	sess, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()
	face, err := app.Font("sans", 15)
	if err != nil {
		log.Fatal(err)
	}
	application := app.NewApplication(sess)
	if _, _, err := component.Window(application, app.WindowConfig{
		Title: "counter", AppID: "dev.example.counter",
	}, &Counter{face: face}); err != nil {
		log.Fatal(err)
	}
	if err := application.Run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

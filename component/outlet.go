package component

import "github.com/stubbedev/gelm/internal/mailbox"

type outlet[Out any] struct {
	loop    Loop
	box     mailbox.Box[Out]
	forward func(Out)
}

func (o *outlet[Out]) send(msg Out) {
	if o.box.Put(msg) {
		o.loop.Invoke(o.drain)
	}
}

func (o *outlet[Out]) drain() {
	for _, msg := range o.box.Take() {
		if o.forward != nil {
			o.forward(msg)
		}
	}
}

func (o *outlet[Out]) stop() {
	o.drain()
	o.box.Stop()
}

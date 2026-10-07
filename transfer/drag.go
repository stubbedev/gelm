package transfer

import "slices"

// Action is a drag-and-drop action, a set when offered: what becomes
// of the payload at the source once dropped.
type Action uint8

// Actions, the wl_data_device_manager dnd_action bits.
const (
	ActionNone Action = 0
	// ActionCopy leaves the source's data as it was.
	ActionCopy Action = 1
	// ActionMove means the source deletes its data once the drop
	// finished.
	ActionMove Action = 2
	// ActionAsk lets the destination ask the user (a drop menu).
	ActionAsk Action = 4
)

// Feedback is what a drag's source hears while the drag runs: whether
// the destination under the pointer accepts (and as which mime), and
// the action the compositor negotiated with it - for a drag icon or
// cursor that tells copy from move, or a source that dims what a move
// would take.
type Feedback struct {
	Mime   string // "" while nothing accepts
	Action Action
}

// Accepted reports whether a destination accepts the drag.
func (f Feedback) Accepted() bool { return f.Mime != "" }

// Drag is a drag's payload and its source-side hooks.
type Drag struct {
	Content
	// Actions are the actions offered; zero offers copy.
	Actions Action
	// OnFeedback, optional, hears every change of acceptance and
	// negotiated action.
	OnFeedback func(Feedback)
	// OnDone, optional, fires once the drag concluded: the action the
	// destination finished with (ActionMove: delete the source data),
	// or ActionNone for a cancelled or rejected drag.
	OnDone func(done Action)
}

// Offered is the action set the drag offers, copy when none was set.
func (d Drag) Offered() Action {
	if d.Actions == ActionNone {
		return ActionCopy
	}
	return d.Actions
}

// Prefer picks the action to take from offered: the first of prefs
// offered, else copy or move (copy first), else none.
func Prefer(offered Action, prefs ...Action) Action {
	for _, p := range slices.Concat(prefs, []Action{ActionCopy, ActionMove}) {
		if offered&p != 0 {
			return p
		}
	}
	return ActionNone
}

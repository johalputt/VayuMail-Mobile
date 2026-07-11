package widgets

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// ListActionKind describes what the user did to a row.
type ListActionKind int

// Row actions.
const (
	// ActionOpen: the row was tapped.
	ActionOpen ListActionKind = iota
	// ActionArchive: right swipe past threshold.
	ActionArchive
	// ActionDelete: left swipe past threshold.
	ActionDelete
)

// ListAction is one user interaction with a message row this frame.
type ListAction struct {
	Kind    ListActionKind
	Message store.Message
}

// Entrance cascade: only the first screenful animates, and only once
// per folder change — scrolling never re-triggers it.
const (
	entranceRows = 10
	entranceStep = 22 * time.Millisecond
	entranceDur  = 240 * time.Millisecond
)

// MessageList is the virtualized inbox list: fixed 72pt rows, only the
// visible range plus a small buffer is laid out (layout.List computes the
// visible window from the scroll offset; off-screen rows are never
// measured or drawn).
type MessageList struct {
	list layout.List
	rows []rowState
	// Swipe enables archive/delete gestures (disabled in search results).
	Swipe bool

	entranceKey   string
	entranceStart time.Time
	entranceLive  bool
}

type rowState struct {
	click widget.Clickable
	swipe Swipeable
}

// NewMessageList constructs a vertical list with swipe enabled.
func NewMessageList() *MessageList {
	return &MessageList{
		list:  layout.List{Axis: layout.Vertical},
		Swipe: true,
	}
}

// BeginEntrance arms the staggered entrance cascade when key (the
// folder identity) changes; same-key calls are no-ops so refreshes
// don't replay it.
func (ml *MessageList) BeginEntrance(key string, now time.Time) {
	if key == ml.entranceKey {
		return
	}
	ml.entranceKey = key
	ml.entranceStart = now
	ml.entranceLive = true
	ml.list.Position.First = 0
	ml.list.Position.Offset = 0
}

// AtTop reports whether the list is scrolled to its very top — the
// arming condition for pull-to-refresh.
func (ml *MessageList) AtTop() bool {
	return ml.list.Position.First == 0 && ml.list.Position.Offset == 0
}

// Layout renders the list and returns any actions performed this frame.
func (ml *MessageList) Layout(gtx layout.Context, th *theme.Theme, msgs []store.Message) []ListAction {
	if len(ml.rows) < len(msgs) {
		ml.rows = append(ml.rows, make([]rowState, len(msgs)-len(ml.rows))...)
	}
	var actions []ListAction

	ml.list.Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
		msg := msgs[i]
		row := &ml.rows[i]

		if row.click.Clicked(gtx) {
			actions = append(actions, ListAction{Kind: ActionOpen, Message: msg})
		}

		rowWidget := func(gtx layout.Context) layout.Dimensions {
			return row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return messageRow(gtx, th, msg, row.click.Pressed())
			})
		}

		var dims layout.Dimensions
		if ml.Swipe {
			var result SwipeResult
			result, dims = row.swipe.Layout(gtx, th, ml.entranceWrap(rowWidget, i))
			switch result {
			case SwipeArchive:
				actions = append(actions, ListAction{Kind: ActionArchive, Message: msg})
			case SwipeDelete:
				actions = append(actions, ListAction{Kind: ActionDelete, Message: msg})
			case SwipeTap:
				// The drag gesture consumes taps before the row's Clickable,
				// so a tap arrives here as SwipeTap — treat it as open.
				actions = append(actions, ListAction{Kind: ActionOpen, Message: msg})
			}
		} else {
			dims = ml.entranceWrap(rowWidget, i)(gtx)
		}

		sepGtx := gtx
		sepGtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
		sep := Separator(sepGtx, th, theme.SeparatorInset)
		return layout.Dimensions{Size: image.Pt(dims.Size.X, dims.Size.Y+sep.Size.Y)}
	})
	return actions
}

// entranceWrap applies the cascade fade+rise to row i while the
// entrance is live; settled rows pay nothing.
func (ml *MessageList) entranceWrap(w layout.Widget, i int) layout.Widget {
	if !ml.entranceLive || i >= entranceRows {
		return w
	}
	return func(gtx layout.Context) layout.Dimensions {
		t, done := anim.Stagger(gtx.Now, ml.entranceStart, i, entranceStep, entranceDur, anim.OutCubic)
		if i == entranceRows-1 && done {
			ml.entranceLive = false
		}
		if done {
			return w(gtx)
		}
		gtx.Execute(op.InvalidateCmd{})

		macro := op.Record(gtx.Ops)
		dims := w(gtx)
		call := macro.Stop()

		rise := int((1 - t) * float32(gtx.Dp(theme.MD)))
		defer op.Offset(image.Pt(0, rise)).Push(gtx.Ops).Pop()
		defer paint.PushOpacity(gtx.Ops, t).Pop()
		call.Add(gtx.Ops)
		return dims
	}
}

// messageRow draws one fixed-height row:
//
//	|bar| [16] [Avatar 40] [12] [ sender ... time / subject / preview ... dot ] [16]
//
// Unread rows carry a 3dp accent bar on the leading edge and full-strength
// text; read rows recede to OnSurface/Subtle.
func messageRow(gtx layout.Context, th *theme.Theme, msg store.Message, pressed bool) layout.Dimensions {
	height := gtx.Dp(theme.RowHeight)
	width := gtx.Constraints.Max.X
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	p := th.Palette

	if pressed {
		paint.FillShape(gtx.Ops, p.Surface, clip.Rect{Max: image.Pt(width, height)}.Op())
	}
	if !msg.IsRead {
		bar := gtx.Dp(3)
		paint.FillShape(gtx.Ops, p.Accent, clip.UniformRRect(
			image.Rect(0, gtx.Dp(theme.MD), bar, height-gtx.Dp(theme.MD)), bar/2).Op(gtx.Ops))
	}

	return layout.Inset{Left: theme.MD, Right: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return Avatar(gtx, th, msg.FromName, msg.FromAddr)
				}),
				layout.Rigid(layout.Spacer{Width: theme.SM + theme.XS}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return rowLine1(gtx, th, msg)
						}),
						layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return rowLine2(gtx, th, msg)
						}))
				}))
		})
}

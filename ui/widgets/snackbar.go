package widgets

import (
	"image"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

const (
	snackbarLifetime = 4 * time.Second
	snackbarSlideIn  = 220 * time.Millisecond
	snackbarSlideOut = 120 * time.Millisecond
)

// Snackbar shows one transient status message pinned to the bottom of
// the screen, optionally with an Undo action. When the message expires
// (4s) the commit callback fires; tapping Undo fires the undo callback
// instead. Safe for concurrent Show calls from loader goroutines.
//
// Frames are requested only while the bar is actually sliding; the ~3.75s
// static hold costs zero frames. The expiry is driven by a one-shot timer
// through Wake, not by per-frame polling — a visible toast used to force
// the whole app to repaint at full frame rate for its entire lifetime.
type Snackbar struct {
	// Wake re-renders the window from any goroutine (window.Invalidate).
	// Set once by the app root; nil is tolerated (the slide-out then starts
	// on the next input-driven frame).
	Wake func()

	mu       sync.Mutex
	msg      string
	shownAt  time.Time
	visible  bool
	hiding   bool
	hideAt   time.Time
	onUndo   func()
	onCommit func()
	expiry   *time.Timer
	undoBtn  widget.Clickable
}

// Show displays a message with an Undo affordance. onCommit fires when
// the snackbar expires un-undone; onUndo fires if the user taps Undo.
func (s *Snackbar) Show(msg string, onUndo, onCommit func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A pending action commits immediately when a new snackbar replaces it.
	if s.visible && s.onCommit != nil {
		go s.onCommit()
	}
	s.msg = msg
	s.onUndo = onUndo
	s.onCommit = onCommit
	s.shownAt = time.Now()
	s.visible = true
	s.hiding = false
	// One wake at expiry so the slide-out starts (and commits) on time even
	// when the app is otherwise idle and scheduling no frames.
	if s.expiry != nil {
		s.expiry.Stop()
	}
	if s.Wake != nil {
		s.expiry = time.AfterFunc(snackbarLifetime, s.Wake)
	}
}

// ShowInfo displays a plain status message with no action.
func (s *Snackbar) ShowInfo(msg string) { s.Show(msg, nil, nil) }

// Layout draws the snackbar when visible. It must be called last in the
// screen so it stacks above content.
func (s *Snackbar) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	s.mu.Lock()
	if !s.visible {
		s.mu.Unlock()
		return layout.Dimensions{}
	}
	now := gtx.Now
	if !s.hiding && now.Sub(s.shownAt) >= snackbarLifetime {
		s.hiding = true
		s.hideAt = now
		if s.onCommit != nil {
			commit := s.onCommit
			s.onCommit = nil
			go commit()
		}
	}
	if s.hiding && now.Sub(s.hideAt) >= snackbarSlideOut {
		s.visible = false
		s.mu.Unlock()
		return layout.Dimensions{}
	}
	msg := s.msg
	hasUndo := s.onUndo != nil
	progress := s.slideProgress(now)
	sliding := s.hiding || now.Sub(s.shownAt) < snackbarSlideIn
	s.mu.Unlock()

	// Animate only the slide phases; the static hold renders zero frames
	// (the expiry timer wakes the window when it is time to slide out).
	if sliding {
		gtx.Execute(op.InvalidateCmd{})
	}

	if s.undoBtn.Clicked(gtx) {
		s.mu.Lock()
		undo := s.onUndo
		s.onUndo = nil
		s.onCommit = nil
		s.hiding = true
		s.hideAt = now
		s.mu.Unlock()
		if undo != nil {
			go undo()
		}
	}

	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: theme.MD, Right: theme.MD, Bottom: theme.MD}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				macro := op.Record(gtx.Ops)
				dims := s.layoutBar(gtx, th, msg, hasUndo)
				call := macro.Stop()

				// Slide up from the bottom by animation progress.
				offset := int(float32(dims.Size.Y+gtx.Dp(theme.MD)) * (1 - progress))
				defer op.Offset(image.Pt(0, offset)).Push(gtx.Ops).Pop()
				call.Add(gtx.Ops)
				return dims
			})
	})
}

// slideProgress returns [0,1]: rising during slide-in (the signature
// OutBack — the bar lands with a whisper of overshoot), falling during
// slide-out (ease-in, receding surfaces accelerate away).
func (s *Snackbar) slideProgress(now time.Time) float32 {
	if s.hiding {
		t := anim.Clamp01(float32(now.Sub(s.hideAt)) / float32(snackbarSlideOut))
		return 1 - anim.InQuad(t)
	}
	t := anim.Clamp01(float32(now.Sub(s.shownAt)) / float32(snackbarSlideIn))
	return anim.OutBack(t)
}

func (s *Snackbar) layoutBar(gtx layout.Context, th *theme.Theme, msg string, hasUndo bool) layout.Dimensions {
	r := gtx.Dp(theme.CornerRadius + 4)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			Shadow(gtx, th, gtx.Constraints.Min, theme.CornerRadius+4)
			defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, r).Push(gtx.Ops).Pop()
			return Fill(gtx, th.Palette.OnBackground)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: theme.SM + theme.XS, Bottom: theme.SM + theme.XS, Left: theme.MD, Right: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Body, th.Palette.Background, msg, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !hasUndo {
								return layout.Dimensions{}
							}
							return s.undoBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(theme.SM).Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										return th.Label(gtx, theme.BodyStrong, th.Palette.Accent, "Undo", 1)
									})
							})
						}))
				})
		})
}

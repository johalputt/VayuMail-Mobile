package ui

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Boot owns the window event loop from the very first frame. On Android
// the splash screen only clears once a frame is presented, so nothing
// blocking may run before Run starts pumping events — the engine attaches
// asynchronously via Attach, and a fatal init error surfaces on screen
// via Fail instead of freezing the splash. This is the fix for the
// "app opens to a frozen logo" bug: app.DataDir()/SQLite/keystore all run
// off the UI thread now (see cmd/vayumail).
type Boot struct {
	ctx    context.Context
	window *app.Window
	th     *theme.Theme
	ops    op.Ops

	ready chan bootResult
	start time.Time

	ui  *UI
	db  *store.DB
	mgr *syncmanager.Manager
	err string
}

type bootResult struct {
	ui    *UI
	db    *store.DB
	mgr   *syncmanager.Manager
	err   error
	stage string
}

// NewBoot prepares the boot loop. The light palette is used for the
// splash until the engine reports the platform preference.
func NewBoot(ctx context.Context, window *app.Window) *Boot {
	return &Boot{
		ctx:    ctx,
		window: window,
		th:     theme.New(false),
		ready:  make(chan bootResult, 1),
	}
}

// Attach hands the initialized engine and UI to the boot loop.
func (b *Boot) Attach(ui *UI, db *store.DB, mgr *syncmanager.Manager) {
	b.ready <- bootResult{ui: ui, db: db, mgr: mgr}
	b.window.Invalidate()
}

// Fail reports a fatal initialization error; the boot screen displays it
// instead of an eternal splash.
func (b *Boot) Fail(err error, stage string) {
	b.ready <- bootResult{err: err, stage: stage}
	b.window.Invalidate()
}

// Run is the single window event loop: it renders the animated brand
// frame until the engine attaches, then delegates every frame to the UI.
func (b *Boot) Run() error {
	for {
		switch e := b.window.Event().(type) {
		case app.FrameEvent:
			if b.start.IsZero() {
				b.start = e.Now
			}
			select {
			case r := <-b.ready:
				if r.err != nil {
					b.err = fmt.Sprintf("Could not start while %s:\n%v", r.stage, r.err)
				} else {
					b.ui, b.db, b.mgr = r.ui, r.db, r.mgr
				}
			default:
			}
			gtx := app.NewContext(&b.ops, e)
			if b.ui != nil {
				b.ui.Frame(gtx)
			} else {
				b.frame(gtx)
			}
			e.Frame(&b.ops)

		case app.DestroyEvent:
			return e.Err
		}
	}
}

// Shutdown releases whatever the boot loop ended up owning.
func (b *Boot) Shutdown() {
	if b.mgr != nil {
		b.mgr.Shutdown()
	}
	if b.db != nil {
		if err := b.db.Close(); err != nil {
			slog.Error("close store", "err", err)
		}
	}
}

// frame draws the splash: the brand mark animates in and gently breathes
// while the engine loads; a fatal error replaces the status line.
func (b *Boot) frame(gtx layout.Context) {
	widgets.FillMax(gtx, b.th.Palette.Background)

	elapsed := gtx.Now.Sub(b.start)
	intro, breathe := bootAnim(elapsed)
	// Keep animating until the engine attaches.
	gtx.Execute(op.InvalidateCmd{})

	markAlpha := uint8(255 * intro * breathe)
	textAlpha := uint8(255 * clamp((float32(elapsed.Milliseconds())-250)/350))

	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return drawBrandMark(gtx, b.th, 92, markAlpha, 0.82+0.18*intro)
			}),
			layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				c := theme.WithAlpha(b.th.Palette.OnBackground, textAlpha)
				return b.th.LabelAligned(gtx, theme.Heading, c, "vayumail", text.Middle)
			}),
			layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if b.err != "" {
					return b.th.Label(gtx, theme.Caption, b.th.Palette.Destructive, b.err, 0)
				}
				c := theme.WithAlpha(b.th.Palette.Subtle, textAlpha)
				return b.th.LabelAligned(gtx, theme.Caption, c, "starting…", text.Middle)
			}))
	})
}

// bootAnim returns the intro progress (0→1, ease-out over 450ms) and a
// breathing multiplier (0.72↔1 sine, ~1.7s period) applied once the intro
// settles.
func bootAnim(elapsed time.Duration) (intro, breathe float32) {
	ms := float32(elapsed.Milliseconds())
	t := clamp(ms / 450)
	intro = 1 - (1-t)*(1-t)*(1-t)
	breathe = 1
	if t >= 1 {
		phase := float64(ms-450) / 1700 * 2 * math.Pi
		breathe = 0.86 + 0.14*float32(math.Sin(phase))
	}
	return intro, breathe
}

func clamp(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// drawBrandMark renders the VayuMail V (assets/logo/vayumail-icon.svg
// geometry) at sizeDp, tinted by alpha and scaled about its center. Gio
// strokes are round-capped, matching the SVG.
func drawBrandMark(gtx layout.Context, th *theme.Theme, sizeDp int, alpha uint8, scale float32) layout.Dimensions {
	px := gtx.Dp(unit.Dp(sizeDp))
	s := float32(px) / 64.0
	ink := theme.WithAlpha(th.Palette.OnBackground, alpha)

	center := f32.Pt(float32(px)/2, float32(px)/2)
	defer op.Affine(f32.Affine2D{}.Scale(center, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()

	pt := func(x, y float32) f32.Point { return f32.Pt(x*s, y*s) }
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(pt(20, 16))
	p.LineTo(pt(29, 42))
	p.MoveTo(pt(46, 13))
	p.CubeTo(pt(42, 26), pt(36, 37), pt(29, 44))
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: p.End(), Width: 10 * s}.Op())

	return layout.Dimensions{Size: image.Pt(px, px)}
}

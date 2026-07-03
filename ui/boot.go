package ui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"sync"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// logoLightPNG is the original VayuMail logo (mark + wordmark, black on
// transparent), shown on the light splash background. It is the exact
// artwork from assets/logo/vayumail.png — not a redrawn approximation.
//
//go:embed logo-light.png
var logoLightPNG []byte

var (
	logoOnce sync.Once
	logoImg  image.Image
)

// brandLogo decodes the embedded logo once and returns it.
func brandLogo() image.Image {
	logoOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(logoLightPNG))
		if err != nil {
			slog.Error("decode splash logo", "err", err)
			return
		}
		logoImg = img
	})
	return logoImg
}

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

// Run is the single window event loop: it renders the static brand
// frame until the engine attaches, then delegates every frame to the UI.
func (b *Boot) Run() error {
	for {
		switch e := b.window.Event().(type) {
		case app.FrameEvent:
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

// frame draws the splash: the original logo (mark + wordmark) shown
// statically, and a status line ("starting…" or, on failure, the fatal
// error). No animation — the logo is presented exactly as provided while
// the engine loads. The single InvalidateCmd only keeps the loop
// repainting so it can notice when the engine finishes attaching; it
// produces no visible motion.
func (b *Boot) frame(gtx layout.Context) {
	widgets.FillMax(gtx, b.th.Palette.Background)
	gtx.Execute(op.InvalidateCmd{})

	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return drawBrandLogo(gtx, 200)
			}),
			layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if b.err != "" {
					return b.th.Label(gtx, theme.Caption, b.th.Palette.Destructive, b.err, 0)
				}
				return b.th.LabelAligned(gtx, theme.Caption, b.th.Palette.Subtle, "starting…", text.Middle)
			}))
	})
}

// drawBrandLogo paints the embedded original logo, scaled to widthDp and
// centered, preserving its aspect ratio. It draws the real PNG artwork —
// no vector reconstruction.
func drawBrandLogo(gtx layout.Context, widthDp int) layout.Dimensions {
	img := brandLogo()
	if img == nil {
		return layout.Dimensions{}
	}
	w := gtx.Dp(unit.Dp(widthDp))
	src := img.Bounds().Dx()
	if src == 0 {
		return layout.Dimensions{}
	}
	scale := float32(w) / float32(src)
	h := int(float32(img.Bounds().Dy()) * scale)

	defer op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	imgOp := paint.NewImageOp(img)
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(w, h)}
}

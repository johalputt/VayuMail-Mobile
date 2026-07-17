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
	"gioui.org/io/event"
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
	logoOp   paint.ImageOp
	logoSize image.Point
)

// brandLogoOp decodes the embedded logo once and returns a cached
// paint.ImageOp. Building the op per frame re-copied the bitmap and minted
// a fresh GPU-texture handle every time, re-uploading the texture on every
// splash frame; a cached op uploads once and is free afterwards.
func brandLogoOp() (paint.ImageOp, image.Point) {
	logoOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(logoLightPNG))
		if err != nil {
			slog.Error("decode splash logo", "err", err)
			return
		}
		logoOp = paint.NewImageOp(img)
		logoSize = img.Bounds().Size()
	})
	return logoOp, logoSize
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

	// listenEvents, if set, is forwarded every window event before the boot
	// loop handles it — used by the file-picker (explorer) to observe the
	// view lifecycle and activity results.
	listenEvents func(event.Event)
}

// SetEventListener registers a callback that receives every window event (in
// addition to the boot loop's own handling). Must be called before Run.
func (b *Boot) SetEventListener(fn func(event.Event)) { b.listenEvents = fn }

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
		evt := b.window.Event()
		if b.listenEvents != nil {
			b.listenEvents(evt)
		}
		switch e := evt.(type) {
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
// error). No animation, and no per-frame invalidation: Attach and Fail
// both call window.Invalidate, so the splash renders a handful of frames
// during exactly the phase where the CPU is busiest (DB open, keystore,
// sync start) instead of spinning at full frame rate.
func (b *Boot) frame(gtx layout.Context) {
	widgets.FillMax(gtx, b.th.Palette.Background)

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
// no vector reconstruction — through the cached ImageOp, so the GPU
// texture is uploaded once for the process lifetime.
func drawBrandLogo(gtx layout.Context, widthDp int) layout.Dimensions {
	imgOp, size := brandLogoOp()
	if size.X == 0 {
		return layout.Dimensions{}
	}
	w := gtx.Dp(unit.Dp(widthDp))
	scale := float32(w) / float32(size.X)
	h := int(float32(size.Y) * scale)

	defer op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(w, h)}
}

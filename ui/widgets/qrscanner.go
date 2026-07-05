package widgets

import (
	"image"
	"image/color"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

const (
	qrFrameSize    = unit.Dp(240)
	qrCornerLen    = unit.Dp(20)
	qrCornerStroke = unit.Dp(2)
	qrFlashFor     = 200 * time.Millisecond
)

// FrameSource supplies camera frames for decoding. It is provided by
// platform code (internal/camera); returning nil means no frame is
// available yet, in which case the scanner shows its paste-code fallback.
//
// Android now has a real frame source: a pure-cgo NDK Camera2 bridge
// (internal/camera, camera_android.go) that streams the luminance plane
// here. It is compiled only by the Android toolchain and verified
// on-device; desktop/CI builds get the no-op source, so the UI, decode
// pipeline, and payload verification remain fully testable here. iOS is
// still pending (COMPLIANCE-TRACKER.md: "Camera preview bridge").
type FrameSource func() image.Image

// QRScanner is the full-screen scanning surface: camera preview (when a
// FrameSource is registered), a 240pt corner-accented frame, a scrim, and
// a single instruction label.
type QRScanner struct {
	source    FrameSource
	decoded   string
	decodedAt time.Time
	reader    gozxing.Reader
}

// NewQRScanner constructs a scanner. source may be nil (no camera).
func NewQRScanner(source FrameSource) *QRScanner {
	return &QRScanner{source: source, reader: qrcode.NewQRCodeReader()}
}

// DecodeImage decodes one image and returns the QR payload text. Exposed
// for the provisioning flow and tests; used internally on camera frames.
func DecodeImage(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	result, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return "", err
	}
	return result.GetText(), nil
}

// Layout draws the scanner and returns the decoded payload once, right
// after the success flash completes.
func (q *QRScanner) Layout(gtx layout.Context, th *theme.Theme) (payload string, done bool) {
	// Try one decode per frame while scanning.
	if q.decoded == "" && q.source != nil {
		if img := q.source(); img != nil {
			if text, err := DecodeImage(img); err == nil && text != "" {
				q.decoded = text
				q.decodedAt = gtx.Now
			}
		}
		gtx.Execute(op.InvalidateCmd{})
	}

	// Success flash, then hand off.
	if q.decoded != "" {
		if gtx.Now.Sub(q.decodedAt) >= qrFlashFor {
			payload, done = q.decoded, true
			q.decoded = ""
		} else {
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	q.drawOverlay(gtx, th)
	return payload, done
}

// drawOverlay renders the scrim, the corner-accented frame, and the
// instruction label.
func (q *QRScanner) drawOverlay(gtx layout.Context, th *theme.Theme) {
	size := gtx.Constraints.Max
	frame := gtx.Dp(qrFrameSize)
	fx := (size.X - frame) / 2
	fy := (size.Y - frame) / 2
	frameRect := image.Rect(fx, fy, fx+frame, fy+frame)

	// 40% black scrim outside the frame: four rectangles so the frame
	// area itself stays clear (and touchable).
	scrim := color.NRGBA{A: 0x66}
	for _, r := range []image.Rectangle{
		{Min: image.Pt(0, 0), Max: image.Pt(size.X, frameRect.Min.Y)},
		{Min: image.Pt(0, frameRect.Max.Y), Max: size},
		{Min: image.Pt(0, frameRect.Min.Y), Max: image.Pt(frameRect.Min.X, frameRect.Max.Y)},
		{Min: image.Pt(frameRect.Max.X, frameRect.Min.Y), Max: image.Pt(size.X, frameRect.Max.Y)},
	} {
		paint.FillShape(gtx.Ops, scrim, clip.Rect(r).Op())
	}

	// Corner accents: 20pt legs, 2pt stroke, square caps, Accent color —
	// flashing green for 200ms after a successful decode.
	col := th.Palette.Accent
	if q.decoded != "" {
		col = theme.ScanSuccess()
	}
	leg := float32(gtx.Dp(qrCornerLen))
	stroke := float32(gtx.Dp(qrCornerStroke))
	var p clip.Path
	p.Begin(gtx.Ops)
	minX, minY := float32(frameRect.Min.X), float32(frameRect.Min.Y)
	maxX, maxY := float32(frameRect.Max.X), float32(frameRect.Max.Y)
	// Top-left, top-right, bottom-right, bottom-left.
	p.MoveTo(f32.Pt(minX, minY+leg))
	p.LineTo(f32.Pt(minX, minY))
	p.LineTo(f32.Pt(minX+leg, minY))
	p.MoveTo(f32.Pt(maxX-leg, minY))
	p.LineTo(f32.Pt(maxX, minY))
	p.LineTo(f32.Pt(maxX, minY+leg))
	p.MoveTo(f32.Pt(maxX, maxY-leg))
	p.LineTo(f32.Pt(maxX, maxY))
	p.LineTo(f32.Pt(maxX-leg, maxY))
	p.MoveTo(f32.Pt(minX+leg, maxY))
	p.LineTo(f32.Pt(minX, maxY))
	p.LineTo(f32.Pt(minX, maxY-leg))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: stroke}.Op())

	// Instruction label below the frame.
	label := "Point at your VayuMail QR code"
	if q.source == nil {
		label = "Camera unavailable — go back and use “Paste setup code”"
	}
	labelGtx := gtx
	labelGtx.Constraints = layout.Exact(size)
	layout.Center.Layout(labelGtx, func(gtx layout.Context) layout.Dimensions {
		offset := frame/2 + gtx.Dp(theme.LG)
		defer op.Offset(image.Pt(0, offset)).Push(gtx.Ops).Pop()
		return th.Label(gtx, theme.Caption, th.Palette.Subtle, label, 1)
	})
}

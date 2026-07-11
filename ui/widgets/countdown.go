package widgets

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// CountdownRing draws a thin circular timer: a faint full track with a
// bright arc sweeping clockwise from twelve o'clock, its length equal to
// frac (1 = full life remaining, 0 = expired). It is a pure function of
// frac — the caller animates it by requesting frames while frac changes;
// the ring itself holds no state (Rule 5).
func CountdownRing(gtx layout.Context, size unit.Dp, frac float32, track, arc color.NRGBA) layout.Dimensions {
	d := gtx.Dp(size)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	stroke := float32(gtx.Dp(unit.Dp(2)))
	r := float32(d)/2 - stroke
	center := f32.Pt(float32(d)/2, float32(d)/2)

	// Track: full faint circle.
	var tp clip.Path
	tp.Begin(gtx.Ops)
	arcPath(&tp, center, r, 0, 2*math.Pi)
	paint.FillShape(gtx.Ops, track, clip.Stroke{Path: tp.End(), Width: stroke}.Op())

	// Arc: bright remaining portion, clockwise from the top.
	if frac > 0 {
		var ap clip.Path
		ap.Begin(gtx.Ops)
		sweep := float64(frac) * 2 * math.Pi
		arcPath(&ap, center, r, -math.Pi/2, -math.Pi/2+sweep)
		paint.FillShape(gtx.Ops, arc, clip.Stroke{Path: ap.End(), Width: stroke}.Op())
	}
	return layout.Dimensions{Size: image.Pt(d, d)}
}

// arcPath appends a polyline approximation of the arc from angle a0 to a1
// (radians, clockwise as angle increases) centered at c with radius r.
func arcPath(p *clip.Path, c f32.Point, r float32, a0, a1 float64) {
	const segments = 48
	start := f32.Pt(c.X+r*float32(math.Cos(a0)), c.Y+r*float32(math.Sin(a0)))
	p.MoveTo(start)
	for i := 1; i <= segments; i++ {
		a := a0 + (a1-a0)*float64(i)/segments
		p.LineTo(f32.Pt(c.X+r*float32(math.Cos(a)), c.Y+r*float32(math.Sin(a))))
	}
}

// RemainingFraction reports how much of a message's life is left at now,
// given its creation and expiry times, in [0,1]. A zero or past expiry
// reports 0; an expiry with no known start reports full.
func RemainingFraction(created, expires, now time.Time) float32 {
	if expires.IsZero() || !now.Before(expires) {
		return 0
	}
	total := expires.Sub(created)
	if total <= 0 {
		return 1
	}
	remaining := expires.Sub(now)
	f := float32(remaining) / float32(total)
	if f > 1 {
		return 1
	}
	if f < 0 {
		return 0
	}
	return f
}

// FormatRemaining renders the time left until expires as a compact label
// (e.g. "4:32", "58s"). Past expiry yields an empty string.
func FormatRemaining(expires, now time.Time) string {
	if expires.IsZero() || !now.Before(expires) {
		return ""
	}
	secs := int(expires.Sub(now).Seconds())
	if secs >= 60 {
		return fmt.Sprintf("%d:%02d", secs/60, secs%60)
	}
	return fmt.Sprintf("%ds", secs)
}

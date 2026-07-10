// Package widgets contains every hand-rolled UI component. No third-party
// widget library is used; each icon below is a Gio path, so the binary
// carries no icon font or image assets.
package widgets

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Icon identifies one of the built-in path icons.
type Icon int

// The full icon set. Icons past IconRefresh are drawn in
// icons_extra.go to respect the 400-line file cap (Rule 10).
const (
	IconMenu Icon = iota
	IconSearch
	IconCompose
	IconBack
	IconArchive
	IconTrash
	IconAttach
	IconSend
	IconEnvelope
	IconShield
	IconSignature
	IconClock
	IconDownload
	IconRefresh
	IconSettings
	IconLogout
	IconLock
	IconAdd
	IconChevronRight
	IconChevronDown
	IconForward
	IconReply
	IconPerson
	IconBell
	IconKey
	IconCheck
	IconClose
	IconBackspace
	IconEye
	IconEyeOff
	IconFolder
)

// DrawIcon renders an icon at the given size, stroked in c. All icons are
// drawn on a 24x24 grid and scaled.
func DrawIcon(gtx layout.Context, icon Icon, c color.NRGBA, size unit.Dp) layout.Dimensions {
	px := gtx.Dp(size)
	s := float32(px) / 24.0
	stroke := 1.8 * s

	var p clip.Path
	p.Begin(gtx.Ops)
	pt := func(x, y float32) f32.Point { return f32.Pt(x*s, y*s) }

	switch icon {
	case IconMenu:
		for _, y := range []float32{7, 12, 17} {
			p.MoveTo(pt(4, y))
			p.LineTo(pt(20, y))
		}
	case IconSearch:
		circle(&p, pt(11, 11), 6*s)
		p.MoveTo(pt(15.5, 15.5))
		p.LineTo(pt(20, 20))
	case IconCompose:
		p.MoveTo(pt(5, 19))
		p.LineTo(pt(9, 18))
		p.LineTo(pt(19, 8))
		p.LineTo(pt(16, 5))
		p.LineTo(pt(6, 15))
		p.LineTo(pt(5, 19))
	case IconBack:
		p.MoveTo(pt(14, 6))
		p.LineTo(pt(8, 12))
		p.LineTo(pt(14, 18))
	case IconArchive:
		p.MoveTo(pt(4, 8))
		p.LineTo(pt(4, 19))
		p.LineTo(pt(20, 19))
		p.LineTo(pt(20, 8))
		p.MoveTo(pt(3, 5))
		p.LineTo(pt(21, 5))
		p.LineTo(pt(21, 8))
		p.LineTo(pt(3, 8))
		p.LineTo(pt(3, 5))
		p.MoveTo(pt(10, 12))
		p.LineTo(pt(14, 12))
	case IconTrash:
		p.MoveTo(pt(5, 7))
		p.LineTo(pt(19, 7))
		p.MoveTo(pt(10, 4))
		p.LineTo(pt(14, 4))
		p.MoveTo(pt(7, 7))
		p.LineTo(pt(8, 20))
		p.LineTo(pt(16, 20))
		p.LineTo(pt(17, 7))
	case IconAttach:
		p.MoveTo(pt(17, 7))
		p.LineTo(pt(17, 15))
		p.CubeTo(pt(17, 21), pt(8, 21), pt(8, 15))
		p.LineTo(pt(8, 7))
		p.CubeTo(pt(8, 3), pt(14, 3), pt(14, 7))
		p.LineTo(pt(14, 15))
		p.CubeTo(pt(14, 17.5), pt(11, 17.5), pt(11, 15))
		p.LineTo(pt(11, 8))
	case IconSend:
		p.MoveTo(pt(4, 12))
		p.LineTo(pt(20, 12))
		p.MoveTo(pt(14, 6))
		p.LineTo(pt(20, 12))
		p.LineTo(pt(14, 18))
	case IconEnvelope:
		p.MoveTo(pt(3, 6))
		p.LineTo(pt(21, 6))
		p.LineTo(pt(21, 18))
		p.LineTo(pt(3, 18))
		p.LineTo(pt(3, 6))
		p.MoveTo(pt(3, 7))
		p.LineTo(pt(12, 13))
		p.LineTo(pt(21, 7))
	case IconShield:
		p.MoveTo(pt(12, 3))
		p.LineTo(pt(19, 6))
		p.LineTo(pt(19, 12))
		p.CubeTo(pt(19, 17), pt(15, 20), pt(12, 21))
		p.CubeTo(pt(9, 20), pt(5, 17), pt(5, 12))
		p.LineTo(pt(5, 6))
		p.LineTo(pt(12, 3))
	case IconSignature:
		p.MoveTo(pt(4, 17))
		p.CubeTo(pt(7, 11), pt(8, 9), pt(9, 12))
		p.CubeTo(pt(10, 15), pt(11, 15), pt(13, 11))
		p.MoveTo(pt(4, 20))
		p.LineTo(pt(20, 20))
	case IconClock:
		circle(&p, pt(12, 12), 8*s)
		p.MoveTo(pt(12, 7))
		p.LineTo(pt(12, 12))
		p.LineTo(pt(16, 14))
	case IconDownload:
		p.MoveTo(pt(12, 4))
		p.LineTo(pt(12, 15))
		p.MoveTo(pt(7, 11))
		p.LineTo(pt(12, 16))
		p.LineTo(pt(17, 11))
		p.MoveTo(pt(5, 20))
		p.LineTo(pt(19, 20))
	case IconRefresh:
		// Three-quarter circular arrow, gap and arrowhead at the top-left.
		p.MoveTo(pt(12, 5))
		p.CubeTo(pt(15.9, 5), pt(19, 8.1), pt(19, 12))
		p.CubeTo(pt(19, 15.9), pt(15.9, 19), pt(12, 19))
		p.CubeTo(pt(8.1, 19), pt(5, 15.9), pt(5, 12))
		p.MoveTo(pt(2.5, 10))
		p.LineTo(pt(5, 12))
		p.LineTo(pt(7.5, 10))
	default:
		extraIconPath(&p, icon, pt, s)
	}

	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: stroke}.Op())
	return layout.Dimensions{Size: image.Pt(px, px)}
}

// circle appends a circular subpath centered at center.
func circle(p *clip.Path, center f32.Point, r float32) {
	const k = 0.5523 // cubic approximation constant
	p.MoveTo(f32.Pt(center.X+r, center.Y))
	p.CubeTo(
		f32.Pt(center.X+r, center.Y+k*r),
		f32.Pt(center.X+k*r, center.Y+r),
		f32.Pt(center.X, center.Y+r))
	p.CubeTo(
		f32.Pt(center.X-k*r, center.Y+r),
		f32.Pt(center.X-r, center.Y+k*r),
		f32.Pt(center.X-r, center.Y))
	p.CubeTo(
		f32.Pt(center.X-r, center.Y-k*r),
		f32.Pt(center.X-k*r, center.Y-r),
		f32.Pt(center.X, center.Y-r))
	p.CubeTo(
		f32.Pt(center.X+k*r, center.Y-r),
		f32.Pt(center.X+r, center.Y-k*r),
		f32.Pt(center.X+r, center.Y))
}

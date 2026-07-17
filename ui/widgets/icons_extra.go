package widgets

import (
	"gioui.org/f32"
	"gioui.org/op/clip"
)

// extraIconPath draws the icons added with the 2.0 redesign, in the
// same 24x24 stroke language as icons.go.
func extraIconPath(p *clip.Path, icon Icon, pt func(x, y float32) f32.Point, s float32) {
	switch icon {
	case IconSettings:
		// Three sliders with offset knobs.
		for i, y := range []float32{7, 12, 17} {
			p.MoveTo(pt(4, y))
			p.LineTo(pt(20, y))
			kx := []float32{15, 9, 13}[i]
			circle(p, pt(kx, y), 2.2*s)
		}
	case IconLogout:
		p.MoveTo(pt(14, 4))
		p.LineTo(pt(5, 4))
		p.LineTo(pt(5, 20))
		p.LineTo(pt(14, 20))
		p.MoveTo(pt(10, 12))
		p.LineTo(pt(21, 12))
		p.MoveTo(pt(17, 8))
		p.LineTo(pt(21, 12))
		p.LineTo(pt(17, 16))
	case IconLock:
		p.MoveTo(pt(5, 11))
		p.LineTo(pt(19, 11))
		p.LineTo(pt(19, 21))
		p.LineTo(pt(5, 21))
		p.LineTo(pt(5, 11))
		p.MoveTo(pt(8, 11))
		p.LineTo(pt(8, 7.5))
		p.CubeTo(pt(8, 2.5), pt(16, 2.5), pt(16, 7.5))
		p.LineTo(pt(16, 11))
		p.MoveTo(pt(12, 15))
		p.LineTo(pt(12, 17.5))
	case IconAdd:
		p.MoveTo(pt(12, 5))
		p.LineTo(pt(12, 19))
		p.MoveTo(pt(5, 12))
		p.LineTo(pt(19, 12))
	case IconChevronRight:
		p.MoveTo(pt(10, 6))
		p.LineTo(pt(16, 12))
		p.LineTo(pt(10, 18))
	case IconChevronDown:
		p.MoveTo(pt(6, 10))
		p.LineTo(pt(12, 16))
		p.LineTo(pt(18, 10))
	case IconForward:
		p.MoveTo(pt(4, 18))
		p.CubeTo(pt(4, 11), pt(8, 8), pt(19, 8))
		p.MoveTo(pt(14, 3))
		p.LineTo(pt(19, 8))
		p.LineTo(pt(14, 13))
	case IconReply:
		p.MoveTo(pt(20, 18))
		p.CubeTo(pt(20, 11), pt(16, 8), pt(5, 8))
		p.MoveTo(pt(10, 3))
		p.LineTo(pt(5, 8))
		p.LineTo(pt(10, 13))
	case IconPerson:
		circle(p, pt(12, 8), 4*s)
		p.MoveTo(pt(4, 20))
		p.CubeTo(pt(4, 15), pt(20, 15), pt(20, 20))
	case IconBell:
		p.MoveTo(pt(5, 17))
		p.LineTo(pt(19, 17))
		p.LineTo(pt(17.5, 15))
		p.LineTo(pt(17.5, 10))
		p.CubeTo(pt(17.5, 3.5), pt(6.5, 3.5), pt(6.5, 10))
		p.LineTo(pt(6.5, 15))
		p.LineTo(pt(5, 17))
		p.MoveTo(pt(10, 20))
		p.CubeTo(pt(10.8, 21.3), pt(13.2, 21.3), pt(14, 20))
	case IconKey:
		circle(p, pt(8, 15), 4*s)
		p.MoveTo(pt(11, 12))
		p.LineTo(pt(20, 3))
		p.MoveTo(pt(16, 7))
		p.LineTo(pt(19, 10))
	case IconCheck:
		p.MoveTo(pt(4.5, 12.5))
		p.LineTo(pt(10, 18))
		p.LineTo(pt(19.5, 6.5))
	case IconClose:
		p.MoveTo(pt(6, 6))
		p.LineTo(pt(18, 18))
		p.MoveTo(pt(18, 6))
		p.LineTo(pt(6, 18))
	case IconBackspace:
		p.MoveTo(pt(9, 5))
		p.LineTo(pt(21, 5))
		p.LineTo(pt(21, 19))
		p.LineTo(pt(9, 19))
		p.LineTo(pt(3, 12))
		p.LineTo(pt(9, 5))
		p.MoveTo(pt(11.5, 9.5))
		p.LineTo(pt(16.5, 14.5))
		p.MoveTo(pt(16.5, 9.5))
		p.LineTo(pt(11.5, 14.5))
	case IconEye:
		p.MoveTo(pt(2.5, 12))
		p.CubeTo(pt(7, 5.5), pt(17, 5.5), pt(21.5, 12))
		p.CubeTo(pt(17, 18.5), pt(7, 18.5), pt(2.5, 12))
		circle(p, pt(12, 12), 3*s)
	case IconEyeOff:
		p.MoveTo(pt(2.5, 12))
		p.CubeTo(pt(7, 5.5), pt(17, 5.5), pt(21.5, 12))
		p.CubeTo(pt(17, 18.5), pt(7, 18.5), pt(2.5, 12))
		p.MoveTo(pt(4, 20))
		p.LineTo(pt(20, 4))
	case IconFolder:
		p.MoveTo(pt(3, 6))
		p.LineTo(pt(9, 6))
		p.LineTo(pt(11, 8))
		p.LineTo(pt(21, 8))
		p.LineTo(pt(21, 19))
		p.LineTo(pt(3, 19))
		p.LineTo(pt(3, 6))
	case IconChat:
		// Rounded speech bubble with a tail at the bottom-left.
		p.MoveTo(pt(4, 5))
		p.LineTo(pt(20, 5))
		p.LineTo(pt(20, 16))
		p.LineTo(pt(9, 16))
		p.LineTo(pt(6, 20))
		p.LineTo(pt(6, 16))
		p.LineTo(pt(4, 16))
		p.LineTo(pt(4, 5))
	case IconFingerprint:
		// Concentric fingerprint ridges — three nested arcs over a core.
		p.MoveTo(pt(5, 13))
		p.CubeTo(pt(5, 6), pt(19, 6), pt(19, 13))
		p.MoveTo(pt(8, 14))
		p.CubeTo(pt(8, 9), pt(16, 9), pt(16, 14))
		p.MoveTo(pt(11, 14.5))
		p.CubeTo(pt(11, 12), pt(13, 12), pt(13, 14.5))
		p.MoveTo(pt(7, 18))
		p.LineTo(pt(7, 17))
		p.MoveTo(pt(17, 18))
		p.LineTo(pt(17, 16))
	}
}

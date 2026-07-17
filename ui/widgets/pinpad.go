package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// PinPadAction is what the user pressed on the pad this frame.
type PinPadAction struct {
	// Digit is '0'–'9' when a number key was tapped, else 0.
	Digit rune
	// Backspace deletes the last digit.
	Backspace bool
	// Submit confirms the entered PIN.
	Submit bool
}

// PinPad is the 4x3 numeric keypad of the lock screen: circular keys
// that dip on press with the shared spring feel. Layout order: 1-9,
// submit, 0, backspace.
type PinPad struct {
	keys  [12]widget.Clickable
	press [12]PressScale
}

// Layout draws the pad; canSubmit lights the confirm key.
func (pp *PinPad) Layout(gtx layout.Context, th *theme.Theme, canSubmit bool) (PinPadAction, layout.Dimensions) {
	var action PinPadAction
	labels := []rune{'1', '2', '3', '4', '5', '6', '7', '8', '9', 'S', '0', 'B'}
	for i, r := range labels {
		if pp.keys[i].Clicked(gtx) {
			switch r {
			case 'S':
				if canSubmit {
					action.Submit = true
				}
			case 'B':
				action.Backspace = true
			default:
				action.Digit = r
			}
		}
	}

	rows := make([]layout.FlexChild, 0, 4)
	for row := 0; row < 4; row++ {
		row := row
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			cols := make([]layout.FlexChild, 0, 3)
			for col := 0; col < 3; col++ {
				i := row*3 + col
				cols = append(cols, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(theme.SM).Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return pp.key(gtx, th, i, labels[i], canSubmit)
						})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEvenly}.Layout(gtx, cols...)
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	return action, dims
}

// key draws one circular pad key with the shared spring press-dip.
func (pp *PinPad) key(gtx layout.Context, th *theme.Theme, i int, r rune, canSubmit bool) layout.Dimensions {
	p := th.Palette
	return pp.keys[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return pp.press[i].Layout(gtx, &pp.keys[i], 0.90, func(gtx layout.Context) layout.Dimensions {
			d := gtx.Dp(68)
			size := image.Pt(d, d)
			defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()

			fillGtx := gtx
			fillGtx.Constraints.Min = size
			switch {
			case r == 'S' && canSubmit:
				FillGradient(fillGtx, p.Accent, p.AccentAlt)
			case r == 'S' || r == 'B':
				// Function keys stay transparent until usable.
			default:
				Fill(fillGtx, p.Surface)
			}

			inner := gtx
			inner.Constraints = layout.Exact(size)
			layout.Center.Layout(inner, func(gtx layout.Context) layout.Dimensions {
				switch r {
				case 'S':
					c := p.Subtle
					if canSubmit {
						c = p.OnAccent
					}
					return DrawIcon(gtx, IconCheck, c, 24)
				case 'B':
					return DrawIcon(gtx, IconBackspace, p.OnSurface, 24)
				default:
					return th.Label(gtx, theme.Numeral, p.OnBackground, string(r), 1)
				}
			})
			return layout.Dimensions{Size: size}
		})
	})
}

// PinDots renders the entry indicator: one dot per expected position,
// filled as digits arrive, shaking horizontally on a rejected PIN.
func PinDots(gtx layout.Context, th *theme.Theme, entered int, shake *anim.Anim) layout.Dimensions {
	p := th.Palette
	t, done := shake.Progress(gtx.Now, anim.Linear)
	if !done {
		gtx.Execute(op.InvalidateCmd{})
	}
	offset := int(anim.Shake(t) * float32(gtx.Dp(10)))

	defer op.Offset(image.Pt(offset, 0)).Push(gtx.Ops).Pop()
	slots := entered
	if slots < 4 {
		slots = 4
	}
	children := make([]layout.FlexChild, 0, slots)
	for i := 0; i < slots; i++ {
		filled := i < entered
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(theme.SM).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				d := gtx.Dp(12)
				defer clip.Ellipse{Max: image.Pt(d, d)}.Push(gtx.Ops).Pop()
				fillGtx := gtx
				fillGtx.Constraints.Min = image.Pt(d, d)
				if filled {
					FillGradient(fillGtx, p.Accent, p.AccentAlt)
				} else {
					Fill(fillGtx, p.Separator)
				}
				return layout.Dimensions{Size: image.Pt(d, d)}
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

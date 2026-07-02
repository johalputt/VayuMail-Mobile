package widgets

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// SearchBar is the inline FTS5-backed search field.
type SearchBar struct {
	editor widget.Editor
	last   string
}

// NewSearchBar constructs a single-line search field.
func NewSearchBar() *SearchBar {
	sb := &SearchBar{}
	sb.editor.SingleLine = true
	sb.editor.Submit = true
	return sb
}

// Query returns the current text.
func (sb *SearchBar) Query() string { return sb.editor.Text() }

// Clear empties the field.
func (sb *SearchBar) Clear() {
	sb.editor.SetText("")
	sb.last = ""
}

// Layout draws the bar and reports whether the query changed this frame.
func (sb *SearchBar) Layout(gtx layout.Context, th *theme.Theme) (changed bool, dims layout.Dimensions) {
	dims = layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return Fill(gtx, th.Palette.Surface)
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(theme.SM+theme.XS).Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return DrawIcon(gtx, IconSearch, th.Palette.Subtle, 18)
								}),
								layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return sb.layoutEditor(gtx, th)
								}))
						})
				})
		})
	if q := sb.editor.Text(); q != sb.last {
		sb.last = q
		changed = true
	}
	return changed, dims
}

func (sb *SearchBar) layoutEditor(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if sb.editor.Len() == 0 {
		// Hint text behind the editor.
		th.Label(gtx, theme.Body, th.Palette.Subtle, "Search", 1)
	}
	return sb.editor.Layout(gtx, th.Shaper, font.Font{Weight: theme.Body.Weight},
		theme.Body.Size, theme.ColorOp(gtx, th.Palette.OnBackground),
		theme.ColorOp(gtx, th.Palette.AccentSubtle))
}

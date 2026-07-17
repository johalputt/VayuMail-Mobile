package widgets

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// RowLine builds a row's "subject — snippet" display line. Called once per
// message when the snapshot loads (off the frame loop): the result is the
// text shaper's cache key, so building it per frame defeated the cache.
func RowLine(msg store.Message) string {
	subject := strings.TrimSpace(msg.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	if snip := strings.TrimSpace(msg.Snippet); snip != "" {
		return subject + " — " + snip
	}
	return subject
}

// rowLine1: sender (strong when unread) … timestamp. now is the frame time
// (gtx.Now), never a fresh time.Now() syscall per row.
func rowLine1(gtx layout.Context, th *theme.Theme, msg store.Message, now time.Time) layout.Dimensions {
	c := th.Palette.OnSurface
	timeC := th.Palette.Subtle
	if !msg.IsRead {
		c = th.Palette.OnBackground
		timeC = th.Palette.Accent
	}
	sender := msg.FromName
	if sender == "" {
		sender = msg.FromAddr
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.BodyStrong, c, sender, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.Caption, timeC, RelativeTime(msg.Date, now), 1)
		}))
}

// rowLine2: subject — snippet … [shield] [clip] [unread dot]. Subject and
// snippet share one truncated line so every row keeps its fixed height.
// line is precomputed (RowLine); an empty string falls back to building it.
func rowLine2(gtx layout.Context, th *theme.Theme, msg store.Message, line string) layout.Dimensions {
	p := th.Palette
	if line == "" {
		line = RowLine(msg)
	}
	col := p.OnSurface
	if !msg.IsRead {
		col = p.OnBackground
	}
	trailing := []layout.FlexChild{}
	if msg.PGPStatus != "" {
		trailing = append(trailing, rowIndicator(th, IconShield, p.Success))
	}
	if msg.HasAttachments {
		trailing = append(trailing, rowIndicator(th, IconAttach, p.Subtle))
	}
	if !msg.IsRead {
		trailing = append(trailing, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.SM}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				d := gtx.Dp(theme.UnreadDotSize)
				defer clip.Ellipse{Max: image.Pt(d, d)}.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: p.Unread}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				return layout.Dimensions{Size: image.Pt(d, d)}
			})
		}))
	}
	children := append([]layout.FlexChild{
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			l := widget.Label{MaxLines: 1, Truncator: "…", Alignment: text.Start}
			return l.Layout(gtx, th.Shaper, font.Font{Weight: theme.Body.Weight},
				theme.Body.Size, line, theme.ColorOp(gtx, col))
		}),
	}, trailing...)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
}

// rowIndicator renders a small trailing status icon.
func rowIndicator(th *theme.Theme, icon Icon, c color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: theme.SM}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return DrawIcon(gtx, icon, c, 14)
		})
	})
}

// RelativeTime renders a compact, human timestamp: clock time today,
// weekday within a week, day-month within a year, else day-month-year.
func RelativeTime(t, now time.Time) string {
	t = t.Local()
	if t.IsZero() {
		return ""
	}
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.Sub(t) < 7*24*time.Hour:
		return t.Format("Mon")
	case y1 == y2:
		return t.Format("2 Jan")
	default:
		return t.Format("02/01/06")
	}
}

// unreadBadge renders the small unread-count pill used in the drawer.
func unreadBadge(gtx layout.Context, th *theme.Theme, count int) layout.Dimensions {
	if count == 0 {
		return layout.Dimensions{}
	}
	label := fmt.Sprintf("%d", count)
	if count > 99 {
		label = "99+"
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			r := size.Y / 2
			defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
			return Fill(gtx, th.Palette.AccentSubtle)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.SM, Right: theme.SM, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Micro, th.Palette.Accent, label, 1)
				})
		})
}

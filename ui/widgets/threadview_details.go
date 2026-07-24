package widgets

// threadview_details.go — the per-message disclosure panel: full
// addressing, delivery security, tracking honesty, and size, in the
// spirit of Gmail's "details" dropdown but with VayuMail's on-device
// facts. Split from threadview.go (Rule 10).

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// detailsPanel renders the expanded header card.
func (tv *ThreadView) detailsPanel(gtx layout.Context, th *theme.Theme, msg store.Message) layout.Dimensions {
	p := th.Palette

	row := func(label, value string, col color.NRGBA) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if strings.TrimSpace(value) == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(64)
							return th.Label(gtx, theme.Caption, p.Subtle, label, 1)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, col, value, 0)
						}))
				})
		})
	}

	// Security: what actually protected this message. Transport is
	// always TLS — the engine refuses plaintext by design — so the line
	// distinguishes end-to-end PGP from transport-only.
	secColor, secText := p.OnSurface, "Transport TLS only — content not end-to-end encrypted"
	switch msg.PGPStatus {
	case "encrypted", "signed+encrypted":
		// Encrypted on-device. The "signed" claim is made only when the
		// signature actually verified against a known key (audit M17).
		if msg.PGPSigVerified {
			secColor, secText = p.Success, "PGP end-to-end encrypted and signed by sender (+ transport TLS)"
		} else {
			secColor, secText = p.Success, "PGP end-to-end encrypted (+ transport TLS)"
		}
	case "signed":
		// A bare multipart/signed structure. The detached signature is not
		// verified on this device, so we must NOT assert the sender is
		// authenticated — say only that a signature is present (audit M17).
		secColor, secText = p.Warning, "PGP signature present — not verified on this device"
	}
	trackColor, trackText := p.OnSurface, "No tracking detected"
	if msg.HasTrackers {
		trackColor, trackText = p.Warning, "Tracking pixels detected — blocked, nothing was fetched"
	}

	sender := msg.FromAddr
	if msg.FromName != "" {
		sender = msg.FromName + " <" + msg.FromAddr + ">"
	}
	date := ""
	if !msg.Date.IsZero() {
		date = msg.Date.Local().Format("Mon, 2 Jan 2006 15:04")
	}
	size := ""
	if msg.SizeBytes > 0 {
		size = humanSize(int(msg.SizeBytes))
	}

	return layout.Inset{Top: theme.SM}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min},
					gtx.Dp(theme.CornerRadius+4)).Push(gtx.Ops).Pop()
				return Fill(gtx, p.Surface)
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(theme.MD).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							row("From", sender, p.OnSurface),
							row("To", msg.ToAddrs, p.OnSurface),
							row("Cc", msg.CcAddrs, p.OnSurface),
							row("Date", date, p.OnSurface),
							row("Security", secText, secColor),
							row("Tracking", trackText, trackColor),
							row("Size", size, p.OnSurface))
					})
			})
	})
}

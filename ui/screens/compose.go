package screens

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Compose hosts the shared composer widget.
type Compose struct {
	backBtn widget.Clickable
}

// NewCompose constructs the compose screen.
func NewCompose() *Compose { return &Compose{} }

// Layout renders the composer and handles its actions.
func (s *Compose) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}

	action := widgets.ComposerAction{}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
				"New Message")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			action = env.Composer.Layout(gtx, th)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}))

	if action.AttachRequested {
		// STUB: file picking needs a platform document-picker bridge;
		// tracked in COMPLIANCE-TRACKER.md ("Attachment picker").
		env.Snack.ShowInfo("Attachments arrive in a later release")
	}
	if action.Send {
		s.send(gtx, env)
	}
	return dims
}

func (s *Compose) send(gtx layout.Context, env *Env) {
	acct, ok := env.State.CurrentAccount()
	if !ok {
		env.Snack.ShowInfo("No account configured")
		return
	}
	draft := env.Composer.Draft(acct.DisplayName, acct.EmailAddress)
	// Encrypted-by-default: when every recipient has a known key, turn
	// encryption on automatically.
	if !env.Composer.Encrypt && env.Keyring.HasKeyFor(draft.Recipients()...) {
		env.Composer.Encrypt = true
		env.Snack.ShowInfo("Encrypted — all recipients have keys")
	}
	opts := state.SendOptions{
		Encrypt: env.Composer.Encrypt,
		Sign:    env.Composer.Sign,
		Keyring: env.Keyring,
	}
	env.State.EnqueueDraft(draft, opts)
	env.Composer.Reset()
	env.Nav.Pop(gtx.Now)
}

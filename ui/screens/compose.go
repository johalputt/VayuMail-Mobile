package screens

import (
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// maxAttachBytes caps a single picked attachment. It matches the VayuPress
// server's generous default (50 MB) so what the app accepts, the server sends.
const maxAttachBytes = 50 << 20

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
		s.pickAttachment(env)
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

// pickAttachment opens the platform file picker off the UI thread (the picker
// blocks until the user chooses), reads the file up to the size cap, and adds
// it to the composer. Everything after the pick runs on the goroutine; the
// composer's AddAttachment is concurrency-safe and Invalidate redraws.
func (s *Compose) pickAttachment(env *Env) {
	if env.PickFile == nil {
		env.Snack.ShowInfo("File picker unavailable on this device")
		return
	}
	pick, compose, invalidate := env.PickFile, env.Composer, env.Invalidate
	go func() {
		rc, err := pick()
		if err != nil || rc == nil {
			return
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxAttachBytes+1))
		if err != nil || len(data) == 0 {
			return
		}
		if len(data) > maxAttachBytes {
			data = data[:maxAttachBytes]
		}
		ctype := http.DetectContentType(data)
		compose.AddAttachment(attachmentName(rc, ctype), ctype, data)
		if invalidate != nil {
			invalidate()
		}
	}()
}

// attachmentName uses the reader's filename when it exposes one (desktop
// pickers hand back an *os.File), otherwise derives a name from the detected
// content type — Android's document stream carries no usable name here.
func attachmentName(rc io.ReadCloser, ctype string) string {
	if n, ok := rc.(interface{ Name() string }); ok {
		base := path.Base(strings.TrimSpace(strings.ReplaceAll(n.Name(), "\\", "/")))
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	ext := ".bin"
	if exts, _ := mime.ExtensionsByType(ctype); len(exts) > 0 {
		ext = exts[0]
	}
	return "attachment" + ext
}

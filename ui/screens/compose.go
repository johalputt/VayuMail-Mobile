package screens

import (
	"image"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// maxAttachBytes caps a single picked attachment. It matches the VayuPress
// server's generous default (50 MB) so what the app accepts, the server sends.
const maxAttachBytes = 50 << 20

// sendFlightDuration is how long the paper-plane sweep plays before the
// composer pops (plan Phase 5.2). The draft is enqueued BEFORE the flight
// starts, so this is pure celebration, never latency for the data.
const sendFlightDuration = 380 * time.Millisecond

// Compose hosts the shared composer widget.
type Compose struct {
	backBtn widget.Clickable

	// keyResult carries an async key-fetch outcome to the UI thread;
	// Layout folds it on the next frame (the outcome mailbox pattern).
	mu        sync.Mutex
	keyResult *keyFetchResult

	// send flight: after a send the composer stays up briefly while a
	// paper-plane accent sweeps off-screen; Reset+Pop happen when it lands.
	flight anim.Anim
	flying bool
}

// keyFetchResult is the outcome of an EnsureKeysFor run.
type keyFetchResult struct{ missing []string }

// NewCompose constructs the compose screen.
func NewCompose() *Compose { return &Compose{} }

// Layout renders the composer and handles its actions.
func (s *Compose) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	s.foldKeyResult(env)

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

	if s.flying {
		t, done := s.flight.Progress(gtx.Now, anim.OutQuad)
		if done {
			s.flying = false
			env.Composer.Reset()
			env.Nav.Pop(gtx.Now)
		} else {
			gtx.Execute(op.InvalidateCmd{})
			s.drawSendFlight(gtx, th, t)
		}
	}

	if action.AttachRequested {
		s.pickAttachment(env)
	}
	if action.EncryptRequested {
		s.fetchMissingKeys(env)
	}
	if action.Send {
		s.send(gtx, env)
	}
	return dims
}

// fetchMissingKeys resolves recipient keys the moment encryption is
// turned on: anything missing is looked up on the recipient's own
// server (WKD) right then, so "encrypt" almost always just works.
func (s *Compose) fetchMissingKeys(env *Env) {
	missing := env.State.MissingKeys(env.Composer.RecipientList())
	if len(missing) == 0 {
		return
	}
	env.Snack.ShowInfo("Fetching keys for " + strings.Join(missing, ", ") + "…")
	env.State.EnsureKeysFor(missing, func(stillMissing []string) {
		s.mu.Lock()
		s.keyResult = &keyFetchResult{missing: stillMissing}
		s.mu.Unlock()
		if env.Invalidate != nil {
			env.Invalidate()
		}
	})
}

// foldKeyResult applies a finished key fetch on the UI thread.
func (s *Compose) foldKeyResult(env *Env) {
	s.mu.Lock()
	r := s.keyResult
	s.keyResult = nil
	s.mu.Unlock()
	if r == nil {
		return
	}
	if len(r.missing) == 0 {
		env.Snack.ShowInfo("All recipients have keys — encrypted")
		return
	}
	env.Composer.Encrypt = false
	env.Snack.ShowInfo("No published key for " + strings.Join(r.missing, ", ") + " — encryption off")
}

func (s *Compose) send(gtx layout.Context, env *Env) {
	if s.flying {
		return // one flight at a time; the button is dead until landing
	}
	acct, ok := env.State.CurrentAccount()
	if !ok {
		env.Snack.ShowInfo("No account configured")
		return
	}
	draft := env.Composer.Draft(acct.DisplayName, acct.EmailAddress)
	// Encryption on but keys missing: name the recipients and hold the
	// send instead of failing later with a generic error.
	if env.Composer.Encrypt {
		if missing := env.State.MissingKeys(draft.Recipients()); len(missing) > 0 {
			env.Snack.ShowInfo("Can't encrypt — no key for " + strings.Join(missing, ", "))
			return
		}
	}
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
	// Send flight (plan Phase 5.2): the message is already queued; the
	// composer stays up for the sweep, then resets and pops when the plane
	// lands. Under reduce-motion the animation snaps done on the first
	// frame, so the pop is immediate.
	s.flight.Start(gtx.Now, sendFlightDuration)
	s.flying = true
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
		defer func() { _ = rc.Close() }()
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

// drawSendFlight paints the paper-plane accent sweeping from the send
// button's corner toward the top edge while the composer finishes its
// send (plan Phase 5.2). t is the eased progress in [0,1). The plane
// rises on a slight arc, shrinks as it departs, and fades out over the
// last third — celebration, not decoration that outstays its welcome.
func (s *Compose) drawSendFlight(gtx layout.Context, th *theme.Theme, t float32) {
	w := float32(gtx.Constraints.Max.X)
	h := float32(gtx.Constraints.Max.Y)

	// Arc: linear across, eased rise with a small overshoot dip.
	x := w - float32(gtx.Dp(56)) - (w*0.6)*t
	y := h - float32(gtx.Dp(96)) - (h*0.55)*t*(2-t) // ease-out vertical
	fade := float32(1)
	if t > 0.7 {
		fade = 1 - (t-0.7)/0.3
	}
	scale := 1 - 0.35*t

	col := th.Palette.Accent
	col.A = uint8(float32(col.A) * fade)

	px := gtx.Dp(26 * unit.Dp(scale))
	defer op.Offset(image.Pt(int(x)-px/2, int(y)-px/2)).Push(gtx.Ops).Pop()
	widgets.DrawIcon(gtx, widgets.IconSend, col, unit.Dp(float32(26)*scale))
}

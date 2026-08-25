package state

// prefsetters.go — preference toggles that outgrew lockstate.go's 400-line
// ceiling (Constitution Rule 10). Each setter applies its effect at once
// where one exists (the reduce-motion gate is global state), then
// persists on a goroutine and refreshes the snapshot.

import (
	"context"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/anim"
)

// SetMotionReduced persists the reduce-motion accessibility toggle and
// applies it immediately — the gate is global, so in-flight animations
// settle on their next frame.
func (s *AppState) SetMotionReduced(reduced bool) {
	anim.SetMotionEnabled(!reduced)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v := "0"
		if reduced {
			v = "1"
		}
		if err := s.db.SetSetting(ctx, store.SettingReduceMotion, v); err != nil {
			s.notify("Could not save setting")
			return
		}
		s.Refresh()
	}()
}

// SetRichHTML persists the rich-HTML rendering opt-in.
func (s *AppState) SetRichHTML(on bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v := "0"
		if on {
			v = "1"
		}
		if err := s.db.SetSetting(ctx, store.SettingRichHTML, v); err != nil {
			s.notify("Could not save setting")
			return
		}
		s.Refresh()
	}()
}

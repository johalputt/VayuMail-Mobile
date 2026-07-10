package screens

// settings_pgp.go — the PGP section: key list with trust cycling,
// import/lookup tools, auto-discovery toggle, and the VayuPress key
// directory. Split from settings.go (Rule 10).

import (
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// pgpRows builds the PGP section.
func (s *Settings) pgpRows(gtx layout.Context, env *Env, snap state.Snapshot) []row {
	th := env.Theme
	p := th.Palette
	rows := []row{s.section(th, "Encryption")}

	// Auto-discovery: on by default; keys arrive from the sender's
	// server (WKD) as mail comes in.
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		inner := s.item(th, "Auto-fetch keys", "Discover contacts' PGP keys from their server (WKD)",
			func(gtx layout.Context) layout.Dimensions {
				dims, toggled := s.autoWKDSwitch.Layout(gtx, th, snap.AutoWKD)
				if toggled {
					env.State.SetAutoWKD(!snap.AutoWKD)
				}
				return dims
			})
		return inner(gtx)
	})

	if len(s.trustBtns) < len(snap.PGPKeys) {
		grow := len(snap.PGPKeys) - len(s.trustBtns)
		s.trustBtns = append(s.trustBtns, make([]widget.Clickable, grow)...)
		s.deleteBtns = append(s.deleteBtns, make([]widget.Clickable, grow)...)
	}
	for i, k := range snap.PGPKeys {
		i, k := i, k
		if s.trustBtns[i].Clicked(gtx) {
			env.State.SetPGPTrust(k.Fingerprint, (k.TrustLevel+1)%3)
		}
		if s.deleteBtns[i].Clicked(gtx) {
			env.State.DeletePGPKey(k.Fingerprint)
		}
		fp := k.Fingerprint
		if len(fp) > 16 {
			fp = fp[:16]
		}
		kind := "public"
		if k.IsPrivate {
			kind = "private"
		}
		rows = append(rows, s.item(th, k.Email, fp+" · "+kind,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.trustBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: theme.MD}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Caption, p.Accent,
										trustLabel(k.TrustLevel), 1)
								})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.deleteBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, p.Destructive, "Remove", 1)
						})
					}))
			}))
	}
	if len(snap.PGPKeys) == 0 {
		rows = append(rows, s.item(th, "No keys yet",
			"Keys are fetched automatically as mail arrives; add one manually below", nil))
	}

	if s.lookupBtn.Clicked(gtx) && strings.Contains(s.keyEmail.Text(), "@") {
		env.State.DiscoverPGPKey(strings.TrimSpace(s.keyEmail.Text()))
	}
	if s.wkdAllBtn.Clicked(gtx) {
		env.State.DiscoverContactKeysWKD()
		env.Snack.ShowInfo("Fetching contacts' keys…")
	}
	if s.importBtn.Clicked(gtx) && strings.Contains(s.keyPaste.Text(), "BEGIN PGP") {
		env.State.ImportPGPKey(s.keyPaste.Text())
		s.keyPaste.SetText("")
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return s.keyTools(gtx, env)
	})

	// VayuPress key directory: auto-import from the operator's site.
	if !s.keyDirLoaded && snap.PGPKeyDirURL != "" {
		s.keyDirEditor.SetText(snap.PGPKeyDirURL)
		s.keyDirLoaded = true
	}
	if s.keyDirSaveBtn.Clicked(gtx) {
		env.State.SetKeyDirectoryURL(s.keyDirEditor.Text())
	}
	if s.keyDirSyncBtn.Clicked(gtx) {
		env.State.SetKeyDirectoryURL(s.keyDirEditor.Text())
		env.State.SyncPGPFromDirectory()
		env.Snack.ShowInfo("Syncing keys from VayuPress…")
	}
	status := "Bulk-import your community's public keys from your VayuPress site"
	if snap.PGPKeyDirURL != "" {
		status = "Directory: " + snap.PGPKeyDirURL
	}
	rows = append(rows, s.item(th, "VayuPress key directory", status, nil))
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return s.keyDirTools(gtx, env)
	})
	return rows
}

// trustLabel names a stored trust level.
func trustLabel(level int) string {
	switch level {
	case 1:
		return "Marginal"
	case 2:
		return "Trusted"
	default:
		return "Unverified"
	}
}

// keyDirTools renders the VayuPress key-directory URL field with Save
// and Sync-keys actions.
func (s *Settings) keyDirTools(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM, Bottom: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if s.keyDirEditor.Len() == 0 {
						th.Label(gtx, theme.Caption, th.Palette.Subtle,
							"https://your-site/.well-known/openpgpkey/", 1)
					}
					return s.keyDirEditor.Layout(gtx, th.Shaper,
						font.Font{Weight: theme.Body.Weight}, theme.Caption.Size,
						theme.ColorOp(gtx, th.Palette.OnBackground),
						theme.ColorOp(gtx, th.Palette.AccentSubtle))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: theme.MD}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.keyDirSaveBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, th.Palette.Accent, "Save", 1)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: theme.MD}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.keyDirSyncBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, th.Palette.Accent, "Sync keys", 1)
						})
					})
				}))
		})
}

// keyTools renders the WKD lookup field and the armored-key paste box.
func (s *Settings) keyTools(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	editor := func(e *widget.Editor, hint string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if e.Len() == 0 {
				th.Label(gtx, theme.Caption, th.Palette.Subtle, hint, 1)
			}
			return e.Layout(gtx, th.Shaper,
				font.Font{Weight: theme.Body.Weight}, theme.Caption.Size,
				theme.ColorOp(gtx, th.Palette.OnBackground),
				theme.ColorOp(gtx, th.Palette.AccentSubtle))
		}
	}
	return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM, Bottom: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, editor(&s.keyEmail, "address for key lookup (WKD)")),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.lookupBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.Accent, "Look up", 1)
							})
						}))
				}),
				layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
				layout.Rigid(editor(&s.keyPaste, "paste armored PGP key…")),
				layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.importBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Caption, th.Palette.Accent, "Import key", 1)
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.wkdAllBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Body, th.Palette.Accent,
							"Fetch contacts' keys now", 1)
					})
				}))
		})
}

package widgets

import (
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// accountHeader renders the drawer's identity block: the active
// account with a large avatar, and — when tapped — an animated reveal
// of the other accounts plus "Add account". Single-account users see
// the reveal too (just the add row), so the affordance is learnable.
func (d *FolderDrawer) accountHeader(gtx layout.Context, th *theme.Theme, accounts []store.Account, currentAcct int64, action *DrawerAction) layout.Dimensions {
	p := th.Palette
	var current store.Account
	for _, a := range accounts {
		if a.ID == currentAcct || current.ID == 0 {
			current = a
		}
		if a.ID == currentAcct {
			break
		}
	}

	if d.headerClick.Clicked(gtx) {
		d.acctExpTarget = !d.acctExpTarget
		d.acctExpanded.Set(d.acctExpTarget, gtx.Now, 200*time.Millisecond)
	}
	t, settled := d.acctExpanded.Progress(gtx.Now, anim.OutCubic)
	if !settled {
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.headerClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XL, Bottom: theme.MD}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return AvatarSized(gtx, th, current.DisplayName, current.EmailAddress, 44)
							}),
							layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										name := current.DisplayName
										if name == "" {
											name = current.EmailAddress
										}
										return th.Label(gtx, theme.BodyStrong, p.OnBackground, name, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return th.Label(gtx, theme.Caption, p.Subtle, current.EmailAddress, 1)
									}))
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								// Chevron rotates 180° as the switcher expands.
								macro := op.Record(gtx.Ops)
								dims := DrawIcon(gtx, IconChevronDown, p.Subtle, 20)
								call := macro.Stop()
								origin := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
								defer op.Affine(f32.Affine2D{}.Rotate(origin, t*3.14159)).Push(gtx.Ops).Pop()
								call.Add(gtx.Ops)
								return dims
							}))
					})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if t == 0 {
				return layout.Dimensions{}
			}
			return d.revealAccounts(gtx, th, accounts, currentAcct, t, action)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Separator(gtx, th, 0)
		}))
}

// revealAccounts clips the switcher list to its animated height.
func (d *FolderDrawer) revealAccounts(gtx layout.Context, th *theme.Theme, accounts []store.Account, currentAcct int64, t float32, action *DrawerAction) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := d.accountRows(gtx, th, accounts, currentAcct, action)
	call := macro.Stop()

	shown := int(float32(dims.Size.Y) * t)
	defer clip.Rect{Max: image.Pt(dims.Size.X, shown)}.Push(gtx.Ops).Pop()
	defer op.Offset(image.Pt(0, shown-dims.Size.Y)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(dims.Size.X, shown)}
}

// accountRows lists the non-active accounts and the add-account row.
func (d *FolderDrawer) accountRows(gtx layout.Context, th *theme.Theme, accounts []store.Account, currentAcct int64, action *DrawerAction) layout.Dimensions {
	p := th.Palette
	var children []layout.FlexChild
	for i := range accounts {
		a := accounts[i]
		if a.ID == currentAcct {
			continue
		}
		click := &d.acctClicks[i]
		if click.Clicked(gtx) {
			action.AccountID = a.ID
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return AvatarSized(gtx, th, a.DisplayName, a.EmailAddress, 28)
							}),
							layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Body, p.OnSurface, a.EmailAddress, 1)
							}))
					})
			})
		}))
	}
	if d.addAcctClick.Clicked(gtx) {
		action.AddAccount = true
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return d.addAcctClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM, Bottom: theme.MD}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return DrawIcon(gtx, IconAdd, p.Accent, 20)
						}),
						layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Body, p.Accent, "Add account", 1)
						}))
				})
		})
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

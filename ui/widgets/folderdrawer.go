package widgets

import (
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

const (
	drawerWidth = unit.Dp(300)
	drawerAnim  = 220 * time.Millisecond
)

// DrawerAction is what the user chose in the drawer this frame.
type DrawerAction struct {
	// FolderID is non-zero when a folder was selected.
	FolderID int64
	// AccountID is non-zero when another account was selected.
	AccountID int64
	// AddAccount is true when "Add account" was tapped.
	AddAccount bool
	// Settings is true when the settings row was tapped.
	Settings bool
	// Talk is true when the VayuTalk row was tapped.
	Talk bool
}

// FolderDrawer is the slide-in navigation panel: account switcher on
// top, folder tree with unread badges, settings at the bottom.
type FolderDrawer struct {
	open      bool
	animStart time.Time

	list          layout.List
	clicks        []widget.Clickable
	settings      widget.Clickable
	talk          widget.Clickable
	scrimClick    widget.Clickable
	headerClick   widget.Clickable
	acctClicks    []widget.Clickable
	addAcctClick  widget.Clickable
	acctExpanded  anim.Bool
	acctExpTarget bool
}

// NewFolderDrawer constructs a closed drawer.
func NewFolderDrawer() *FolderDrawer {
	return &FolderDrawer{list: layout.List{Axis: layout.Vertical}}
}

// Open slides the drawer in.
func (d *FolderDrawer) Open(now time.Time) {
	if d.open {
		return
	}
	d.open = true
	d.animStart = now
}

// Close slides the drawer out.
func (d *FolderDrawer) Close(now time.Time) {
	if !d.open {
		return
	}
	d.open = false
	d.animStart = now
	d.acctExpTarget = false
	d.acctExpanded.Jump(false)
}

// IsOpen reports whether the drawer is open or animating open.
func (d *FolderDrawer) IsOpen() bool { return d.open }

// Layout draws the scrim and panel above the screen content. It returns
// the user's choice, if any, this frame.
func (d *FolderDrawer) Layout(gtx layout.Context, th *theme.Theme, accounts []store.Account, currentAcct int64, folders []store.Folder, unread map[int64]int, currentID int64) DrawerAction {
	progress := d.progress(gtx.Now)
	if progress == 0 && !d.open {
		return DrawerAction{}
	}
	if d.animatingAt(gtx.Now) {
		gtx.Execute(op.InvalidateCmd{})
	}

	var action DrawerAction

	// Scrim fades with progress; tap closes.
	if d.scrimClick.Clicked(gtx) {
		d.Close(gtx.Now)
	}
	d.scrimClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		alpha := uint8(float32(0x73) * progress)
		return FillMax(gtx, theme.WithAlpha(th.Palette.Shadow, alpha))
	})

	if len(d.clicks) < len(folders)+1 {
		d.clicks = append(d.clicks, make([]widget.Clickable, len(folders)+1-len(d.clicks))...)
	}
	if len(d.acctClicks) < len(accounts) {
		d.acctClicks = append(d.acctClicks, make([]widget.Clickable, len(accounts)-len(d.acctClicks))...)
	}

	w := gtx.Dp(drawerWidth)
	offset := int(float32(w) * (progress - 1))
	func() {
		defer op.Offset(image.Pt(offset, 0)).Push(gtx.Ops).Pop()
		panelGtx := gtx
		panelGtx.Constraints = layout.Exact(image.Pt(w, gtx.Constraints.Max.Y))
		r := gtx.Dp(theme.CardRadius)
		defer clip.RRect{
			Rect: image.Rectangle{Max: panelGtx.Constraints.Max},
			NE:   r, SE: r,
		}.Push(gtx.Ops).Pop()
		Fill(panelGtx, th.Palette.SurfaceRaised)
		a := d.layoutPanel(panelGtx, th, accounts, currentAcct, folders, unread, currentID)
		if a != (DrawerAction{}) {
			action = a
			d.Close(gtx.Now)
		}
	}()
	return action
}

// layoutPanel stacks header, folder list, and the settings footer.
func (d *FolderDrawer) layoutPanel(gtx layout.Context, th *theme.Theme, accounts []store.Account, currentAcct int64, folders []store.Folder, unread map[int64]int, currentID int64) DrawerAction {
	var action DrawerAction
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.accountHeader(gtx, th, accounts, currentAcct, &action)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return d.list.Layout(gtx, len(folders)+1, func(gtx layout.Context, i int) layout.Dimensions {
				if i == 0 {
					unifiedID := int64(-1)
					if d.clicks[0].Clicked(gtx) {
						action.FolderID = unifiedID
					}
					f := store.Folder{ID: unifiedID, Name: "All inboxes"}
					return d.folderRow(gtx, th, &d.clicks[0], f,
						unread[unifiedID], currentID == unifiedID)
				}
				f := folders[i-1]
				if d.clicks[i].Clicked(gtx) {
					action.FolderID = f.ID
				}
				return d.folderRow(gtx, th, &d.clicks[i], f, unread[f.ID], f.ID == currentID)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Separator(gtx, th, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.talk.Clicked(gtx) {
				action.Talk = true
			}
			return d.footerRow(gtx, th, &d.talk, IconChat, "VayuTalk", th.Palette.Accent)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.settings.Clicked(gtx) {
				action.Settings = true
			}
			return d.footerRow(gtx, th, &d.settings, IconSettings, "Settings", th.Palette.OnSurface)
		}))
	return action
}

// footerRow draws one tappable icon+label row for the drawer footer.
func (d *FolderDrawer) footerRow(gtx layout.Context, th *theme.Theme, click *widget.Clickable, icon Icon, label string, tint color.NRGBA) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Left: theme.LG, Top: theme.MD, Bottom: theme.MD, Right: theme.LG}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return DrawIcon(gtx, icon, tint, 20)
					}),
					layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Body, tint, label, 1)
					}))
			})
	})
}

// folderRow draws one folder with its icon, an active pill highlight,
// and an unread badge.
func (d *FolderDrawer) folderRow(gtx layout.Context, th *theme.Theme, click *widget.Clickable, f store.Folder, unreadCount int, active bool) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: theme.SM, Right: theme.SM, Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := func(gtx layout.Context) layout.Dimensions {
					if !active {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}
					defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(theme.CornerRadius+4)).Push(gtx.Ops).Pop()
					return Fill(gtx, th.Palette.AccentSubtle)
				}
				return layout.Background{}.Layout(gtx, bg,
					func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.SM + theme.XS, Bottom: theme.SM + theme.XS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								c := th.Palette.OnSurface
								if active {
									c = th.Palette.Accent
								}
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return DrawIcon(gtx, folderIcon(f), c, 20)
									}),
									layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return th.Label(gtx, theme.Body, c, f.Name, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return unreadBadge(gtx, th, unreadCount)
									}))
							})
					})
			})
	})
}

// folderIcon maps a folder's role onto its glyph.
func folderIcon(f store.Folder) Icon {
	switch {
	case f.ID == -1 || f.IsInbox:
		return IconEnvelope
	case f.IsSent:
		return IconSend
	case f.IsDrafts:
		return IconCompose
	case f.IsTrash:
		return IconTrash
	case f.IsArchive:
		return IconArchive
	default:
		return IconFolder
	}
}

// progress returns the drawer's slide progress in [0,1].
func (d *FolderDrawer) progress(now time.Time) float32 {
	t := float32(now.Sub(d.animStart)) / float32(drawerAnim)
	if t > 1 {
		t = 1
	}
	if d.open {
		return anim.OutCubic(t)
	}
	return 1 - anim.OutCubic(t)
}

func (d *FolderDrawer) animatingAt(now time.Time) bool {
	return now.Sub(d.animStart) < drawerAnim
}

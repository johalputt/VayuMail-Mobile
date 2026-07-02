package widgets

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

const (
	drawerWidth = unit.Dp(280)
	drawerAnim  = 200 * time.Millisecond
)

// DrawerAction is what the user chose in the drawer this frame.
type DrawerAction struct {
	// FolderID is non-zero when a folder was selected.
	FolderID int64
	// Settings is true when the settings row was tapped.
	Settings bool
}

// FolderDrawer is the slide-in folder tree with unread badges.
type FolderDrawer struct {
	open      bool
	animStart time.Time
	opening   bool

	list       layout.List
	clicks     []widget.Clickable
	settings   widget.Clickable
	scrimClick widget.Clickable
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
	d.opening = true
	d.animStart = now
}

// Close slides the drawer out.
func (d *FolderDrawer) Close(now time.Time) {
	if !d.open {
		return
	}
	d.open = false
	d.opening = false
	d.animStart = now
}

// IsOpen reports whether the drawer is open or animating open.
func (d *FolderDrawer) IsOpen() bool { return d.open }

// Layout draws the scrim and panel above the screen content. It returns
// the user's choice, if any, this frame.
func (d *FolderDrawer) Layout(gtx layout.Context, th *theme.Theme, folders []store.Folder, unread map[int64]int, currentID int64) DrawerAction {
	progress := d.progress(gtx.Now)
	if progress == 0 && !d.open {
		return DrawerAction{}
	}
	if d.animatingAt(gtx.Now) {
		gtx.Execute(op.InvalidateCmd{})
	}

	var action DrawerAction

	// Scrim: 40% black, fading with progress; tap closes.
	if d.scrimClick.Clicked(gtx) {
		d.Close(gtx.Now)
	}
	d.scrimClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		alpha := uint8(float32(0x66) * progress)
		return FillMax(gtx, theme.WithAlpha(th.Palette.OnBackground, alpha))
	})

	// Panel sliding from the left.
	if len(d.clicks) < len(folders) {
		d.clicks = append(d.clicks, make([]widget.Clickable, len(folders)-len(d.clicks))...)
	}
	w := gtx.Dp(drawerWidth)
	offset := int(float32(w) * (progress - 1))
	func() {
		defer op.Offset(image.Pt(offset, 0)).Push(gtx.Ops).Pop()
		panelGtx := gtx
		panelGtx.Constraints = layout.Exact(image.Pt(w, gtx.Constraints.Max.Y))
		defer clip.Rect{Max: panelGtx.Constraints.Max}.Push(gtx.Ops).Pop()
		Fill(panelGtx, th.Palette.Surface)
		a := d.layoutPanel(panelGtx, th, folders, unread, currentID)
		if a.FolderID != 0 || a.Settings {
			action = a
			d.Close(gtx.Now)
		}
	}()
	return action
}

func (d *FolderDrawer) layoutPanel(gtx layout.Context, th *theme.Theme, folders []store.Folder, unread map[int64]int, currentID int64) DrawerAction {
	var action DrawerAction
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: theme.XL, Left: theme.LG, Bottom: theme.MD}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Heading, th.Palette.OnBackground, "Folders", 1)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return d.list.Layout(gtx, len(folders), func(gtx layout.Context, i int) layout.Dimensions {
				f := folders[i]
				if d.clicks[i].Clicked(gtx) {
					action.FolderID = f.ID
				}
				return d.folderRow(gtx, th, &d.clicks[i], f, unread[f.ID], f.ID == currentID)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.settings.Clicked(gtx) {
				action.Settings = true
			}
			return d.settings.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: theme.LG, Top: theme.MD, Bottom: theme.XL}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Body, th.Palette.OnSurface, "Settings", 1)
					})
			})
		}))
	return action
}

func (d *FolderDrawer) folderRow(gtx layout.Context, th *theme.Theme, click *widget.Clickable, f store.Folder, unreadCount int, active bool) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := func(gtx layout.Context) layout.Dimensions {
			if !active {
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}
			return Fill(gtx, th.Palette.AccentSubtle)
		}
		return layout.Background{}.Layout(gtx, bg,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM + theme.XS, Bottom: theme.SM + theme.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						c := th.Palette.OnSurface
						if active {
							c = th.Palette.Accent
						}
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Body, c, f.Name, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return unreadBadge(gtx, th, unreadCount)
							}))
					})
			})
	})
}

// progress returns the drawer's slide progress in [0,1].
func (d *FolderDrawer) progress(now time.Time) float32 {
	t := float32(now.Sub(d.animStart)) / float32(drawerAnim)
	if t > 1 {
		t = 1
	}
	if d.open {
		return 1 - (1-t)*(1-t)*(1-t) // ease-out in
	}
	inv := 1 - t
	return inv * inv * inv // ease-in out
}

func (d *FolderDrawer) animatingAt(now time.Time) bool {
	return now.Sub(d.animStart) < drawerAnim
}

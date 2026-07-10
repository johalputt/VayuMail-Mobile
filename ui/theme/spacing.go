package theme

import "gioui.org/unit"

// The 4pt spacing grid. Every inset, margin, and gap in the app is one of
// these constants — no ad-hoc values.
const (
	// XS is icon padding and tight internal spacing.
	XS = unit.Dp(4)
	// SM is component internal padding and icon margins.
	SM = unit.Dp(8)
	// MD is section padding and card insets.
	MD = unit.Dp(16)
	// LG is the screen horizontal margin.
	LG = unit.Dp(24)
	// XL separates major sections.
	XL = unit.Dp(32)
	// XXL is empty-state illustration padding.
	XXL = unit.Dp(48)
)

// Component metrics derived from the design spec.
const (
	// RowHeight is the fixed message-list row height (virtualization
	// requires fixed heights).
	RowHeight = unit.Dp(72)
	// AvatarSize is the message-list avatar diameter.
	AvatarSize = unit.Dp(40)
	// UnreadDotSize is the unread indicator diameter.
	UnreadDotSize = unit.Dp(6)
	// SeparatorInset aligns row separators with the text block:
	// LG(16 in-row) + avatar 40 + SM(8).
	SeparatorInset = unit.Dp(64)
	// TouchTarget is the minimum tappable square.
	TouchTarget = unit.Dp(48)
	// CornerRadius is the app-wide control radius.
	CornerRadius = unit.Dp(8)
	// CardRadius rounds raised surfaces: cards, dialogs, sheets.
	CardRadius = unit.Dp(16)
	// PillRadius makes a full pill of any control up to 48dp tall.
	PillRadius = unit.Dp(24)
	// FABSize is the floating compose button diameter.
	FABSize = unit.Dp(56)
	// FABMargin is the FAB's inset from the screen corner.
	FABMargin = unit.Dp(20)
)

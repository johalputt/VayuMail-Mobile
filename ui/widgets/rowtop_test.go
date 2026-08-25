package widgets

import "testing"

// RowTop seeds the thread-open hero rect (plan Phase 5.1). The geometry
// is a contract between layout.List's Position semantics and the morph:
// if these drift, the container grows out of the wrong place.

const heroTestRowH = 144 // 72dp at 2x

func TestRowTopGeometry(t *testing.T) {
	t.Run("at top", func(t *testing.T) {
		if got := RowTop(0, 0, 3, heroTestRowH); got != 3*heroTestRowH {
			t.Fatalf("row 3 from top = %d, want %d", got, 3*heroTestRowH)
		}
	})
	t.Run("scrolled into a row", func(t *testing.T) {
		// First visible row partially scrolled off by 30px.
		if got := RowTop(2, -30, 4, heroTestRowH); got != -30+2*heroTestRowH {
			t.Fatalf("row 4 = %d, want %d", got, -30+2*heroTestRowH)
		}
	})
	t.Run("first row fully above viewport", func(t *testing.T) {
		// Offset == -rowH means the first visible child starts exactly at
		// the top edge; the previous row would sit at -rowH.
		if got := RowTop(5, -heroTestRowH, 4, heroTestRowH); got != -2*heroTestRowH {
			t.Fatalf("row above first visible = %d, want %d", got, -2*heroTestRowH)
		}
	})
}

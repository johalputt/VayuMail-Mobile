package widgets

import (
	"testing"
	"time"
)

func TestRemainingFraction(t *testing.T) {
	created := time.Unix(1000, 0)
	expires := time.Unix(1100, 0) // 100s lifetime

	if f := RemainingFraction(created, expires, time.Unix(1000, 0)); f != 1 {
		t.Errorf("at start = %v, want 1", f)
	}
	if f := RemainingFraction(created, expires, time.Unix(1050, 0)); f < 0.49 || f > 0.51 {
		t.Errorf("at midpoint = %v, want ~0.5", f)
	}
	if f := RemainingFraction(created, expires, time.Unix(1100, 0)); f != 0 {
		t.Errorf("at expiry = %v, want 0", f)
	}
	if f := RemainingFraction(created, expires, time.Unix(1200, 0)); f != 0 {
		t.Errorf("past expiry = %v, want 0", f)
	}
	if f := RemainingFraction(created, time.Time{}, time.Unix(1050, 0)); f != 0 {
		t.Errorf("zero expiry = %v, want 0", f)
	}
}

func TestFormatRemaining(t *testing.T) {
	now := time.Unix(1000, 0)
	cases := []struct {
		expires time.Time
		want    string
	}{
		{time.Unix(1000+272, 0), "4:32"},
		{time.Unix(1000+58, 0), "58s"},
		{time.Unix(1000, 0), ""}, // already expired
		{time.Unix(999, 0), ""},  // in the past
		{time.Time{}, ""},        // no TTL
	}
	for _, c := range cases {
		if got := FormatRemaining(c.expires, now); got != c.want {
			t.Errorf("FormatRemaining(%v) = %q, want %q", c.expires, got, c.want)
		}
	}
}

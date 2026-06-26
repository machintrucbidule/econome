package domain

import "testing"

func TestDateCompare(t *testing.T) {
	a := NewDate(2026, 6, 15)
	b := NewDate(2026, 6, 16)
	c := NewDate(2026, 7, 1)
	d := NewDate(2027, 1, 1)

	if !a.Before(b) || !b.Before(c) || !c.Before(d) {
		t.Error("ordering broken")
	}
	if !b.After(a) {
		t.Error("After broken")
	}
	if NewDate(2026, 1, 1).Compare(NewDate(2026, 1, 1)) != 0 {
		t.Error("equal dates should compare 0")
	}
}

func TestDateZeroAndFormat(t *testing.T) {
	if !(Date{}).IsZero() {
		t.Error("zero value")
	}
	if NewDate(2026, 6, 1).IsZero() {
		t.Error("non-zero")
	}
	if got := NewDate(2026, 6, 5).String(); got != "2026-06-05" {
		t.Errorf("String = %q", got)
	}
	if got := NewDate(2026, 6, 5).Period(); got != "2026-06" {
		t.Errorf("Period = %q", got)
	}
}

package domain

import "fmt"

// Date is a timezone-free calendar date (the engine never touches a clock or a
// timezone; `today` is injected as a Date). Periods are "YYYY-MM" strings.
type Date struct {
	Year  int
	Month int // 1–12
	Day   int // 1–31
}

// NewDate builds a Date.
func NewDate(year, month, day int) Date { return Date{Year: year, Month: month, Day: day} }

// Compare orders dates: -1 if d<o, 0 if equal, +1 if d>o.
func (d Date) Compare(o Date) int {
	switch {
	case d.Year != o.Year:
		return sign(d.Year - o.Year)
	case d.Month != o.Month:
		return sign(d.Month - o.Month)
	default:
		return sign(d.Day - o.Day)
	}
}

// Before reports whether d is strictly before o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// After reports whether d is strictly after o.
func (d Date) After(o Date) bool { return d.Compare(o) > 0 }

// IsZero reports whether d is the zero value (no date).
func (d Date) IsZero() bool { return d == Date{} }

// String renders ISO "YYYY-MM-DD".
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// Period returns the "YYYY-MM" period this date falls in.
func (d Date) Period() string { return fmt.Sprintf("%04d-%02d", d.Year, d.Month) }

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

package view

import (
	"fmt"
	"html/template"
	"strings"
)

// View-models for the read-only Forecast screen (functional/05, increment 6a).
// The handler precomputes every display string; the template stays logic-free
// (G5). Figures are the pure engine's output, formatted here — never recomputed.

// FScope is one rail account-scope entry (aggregated or a current account).
type FScope struct {
	Key   string // "all" or the account id as a string
	Name  string
	Note  string
	On    bool
	IsAll bool
}

// FTxn is one read-only transaction line in a leaf's drill-down (D2).
type FTxn struct {
	Label       string
	DateStr     string
	Approx      bool
	AmountStr   string
	AmountNeg   bool
	StatusClass string
	StatusLabel string
}

// FRow is one rendered table row. Kind ∈ parent | child | leaf | flat (flat =
// the aggregated-scope flat leaf with an account pill).
type FRow struct {
	Kind        string
	Key         string // "e<envID>" (leaf/child/flat) · "c<catID>" (parent)
	EnvID       int64
	ParentKey   string // child rows: the parent's Key (for data-c)
	Name        string
	AccountName string
	ShowPill    bool
	AggBadge    bool
	Income      bool
	PlannedStr  string
	RealStr     string
	RemainStr   string
	RealNeg     bool
	RemainNeg   bool
	RemainDash  bool // income rows show "—" for remaining
	BadgeClass  string
	BadgeLabel  string
	AggLabel    string
	HasBar      bool
	BarPercent  int
	BarClass    string
	Children    []FRow
	Drill       []FTxn
	HasDrill    bool

	// interaction / fragment plumbing (6b)
	Editable bool   // the Prévu cell is an inline input (active month, leaf/child)
	Period   string // for the hx-patch URL
	Scope    string
	Hidden   bool // a child row is collapsed by default
	OOB      bool // render with hx-swap-oob (out-of-band fragment)
}

// FTotal is the footer total (expense only).
type FTotal struct {
	PlannedStr string
	RealStr    string
	RemainStr  string
	RemainNeg  bool
}

// FFig is one right-panel figure card.
type FFig struct {
	Label string
	Value string
	Sub   string
	Mod   string // "" | "hl" | "good" | "bad"
	Help  string // optional tooltip
}

// FEncart is a savings band (residual / negative / cascade).
type FEncart struct {
	Kind        string // residual | negative | cascade
	Title       string
	BigStr      string
	Sub         string
	AccountName string
	ActionLabel string
	SweepID     int64 // the sweep account the "Virer" action posts for
	Disabled    bool  // the end-of-month transfer cannot run (to_save ≤ 0 / locked)
}

// FWatch is one "à surveiller" item.
type FWatch struct {
	Label    string
	ValueStr string
	Bad      bool
}

// ForecastView backs GET /{$} (the budget landing).
type ForecastView struct {
	Base
	Email      string
	Nav        string
	Period     string
	MonthLabel string
	PrevPeriod string
	NextPeriod string
	YearLabel  string
	MonthIndex int // 0-based selected month (picker)
	Scope      string
	ScopeKind  string
	Scopes     []FScope

	PickerOpen     bool
	PrevYearPeriod string
	NextYearPeriod string
	MonthCells     []MonthCell

	NotCreated bool
	Locked     bool
	Empty      bool
	Editable   bool // active month → inline edits enabled
	OOB        bool // render the panel/total fragments out-of-band (PATCH response)

	Rows               []FRow
	ShowPills          bool
	HasHiddenTransfers bool
	Total              FTotal

	Figures        []FFig
	Encarts        []FEncart
	CarryNote      bool
	CarryNext      string
	Watch          []FWatch
	HasWatch       bool
	TimelineTitle  string
	TimelineSVG    template.HTML
	TimelineLegend []TLLegend
}

// MonthCell is one month link in the navigator's year grid (M7).
type MonthCell struct {
	Period string
	Label  string
	On     bool
}

// TLLegend is one treasury-timeline legend entry.
type TLLegend struct {
	Label  string
	Color  string // CSS var name or value
	Hollow bool   // ring style (low point)
}

// TLPoint is one timeline point handed to the SVG renderer.
type TLPoint struct {
	Day     int
	Balance int64
	Kind    string // start | income | debit | awaited | overrun | low
}

// TLInput is the geometry-free input the SVG renderer consumes.
type TLInput struct {
	DaysInMonth int
	Points      []TLPoint
	LowValueStr string
	LowDay      int
	LowBalance  int64
	LowBreaches bool
	EndLabel    string // "point bas" / "fin de mois" / "point bas critique"
}

// timeline SVG geometry (matches the validated mockup viewBox/margins).
const (
	tlVBW    = 1500.0
	tlVBH    = 230.0
	tlXLeft  = 87.0
	tlXRight = 1460.0
	tlYTop   = 40.0
	tlYBot   = 190.0
	tlYBase  = 200.0
)

// RenderTimeline builds the server-rendered treasury-timeline SVG (M17): the
// running-balance polyline + area, event dots coloured by kind, and the
// low-point/end marker. Coordinates are presentation floats; money stays integer
// minor units (only the marker label is a formatted string).
func RenderTimeline(in TLInput) template.HTML {
	if len(in.Points) == 0 {
		return ""
	}
	dim := in.DaysInMonth
	if dim < 1 {
		dim = 30
	}
	minB, maxB := in.Points[0].Balance, in.Points[0].Balance
	for _, p := range in.Points {
		if p.Balance < minB {
			minB = p.Balance
		}
		if p.Balance > maxB {
			maxB = p.Balance
		}
	}
	x := func(day int) float64 {
		if dim <= 1 {
			return tlXLeft
		}
		d := day
		if d < 1 {
			d = 1
		}
		if d > dim {
			d = dim
		}
		return tlXLeft + (float64(d-1)/float64(dim-1))*(tlXRight-tlXLeft)
	}
	y := func(b int64) float64 {
		if maxB == minB {
			return (tlYTop + tlYBot) / 2
		}
		return tlYTop + (float64(maxB-b)/float64(maxB-minB))*(tlYBot-tlYTop)
	}

	var poly strings.Builder
	for i, p := range in.Points {
		if i > 0 {
			poly.WriteByte(' ')
		}
		fmt.Fprintf(&poly, "%.0f,%.0f", x(p.Day), y(p.Balance))
	}
	// Area path: the polyline, then down to the baseline and back.
	var area strings.Builder
	for i, p := range in.Points {
		if i == 0 {
			fmt.Fprintf(&area, "M%.0f,%.0f", x(p.Day), y(p.Balance))
		} else {
			fmt.Fprintf(&area, " L%.0f,%.0f", x(p.Day), y(p.Balance))
		}
	}
	last := in.Points[len(in.Points)-1]
	first := in.Points[0]
	fmt.Fprintf(&area, " L%.0f,%.0f L%.0f,%.0f Z", x(last.Day), tlYBase, x(first.Day), tlYBase)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" role="img">`, tlVBW, tlVBH)
	// grid + baseline
	b.WriteString(`<g stroke="var(--line)" stroke-width="1">`)
	for _, gy := range []float64{tlYTop + 4, (tlYTop+tlYBot)/2 + 4, tlYBot - 26} {
		fmt.Fprintf(&b, `<line x1="40" y1="%.0f" x2="1480" y2="%.0f"/>`, gy, gy)
	}
	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<line x1="40" y1="%.0f" x2="1480" y2="%.0f" stroke="#cdd6da" stroke-width="1.5"/>`, tlYBase, tlYBase)
	// area + line
	fmt.Fprintf(&b, `<path d="%s" fill="rgba(17,104,138,.10)"/>`, area.String())
	fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="var(--brand)" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/>`, poly.String())
	// event dots
	for _, p := range in.Points {
		px, py := x(p.Day), y(p.Balance)
		switch p.Kind {
		case "start", "income":
			fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="5" fill="var(--ok)"/>`, px, py)
		case "overrun":
			fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="5" fill="var(--bad)"/>`, px, py)
		case "awaited":
			fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="4.5" fill="none" stroke="var(--warn)" stroke-width="2" stroke-dasharray="3 2"/>`, px, py)
		default:
			fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="4" fill="var(--brand)"/>`, px, py)
		}
	}
	// low-point / end-of-month marker
	lx, ly := x(in.LowDay), y(in.LowBalance)
	markColor := "var(--ok)"
	if in.LowBreaches {
		markColor = "var(--bad)"
	}
	fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="7" fill="var(--surface)" stroke="%s" stroke-width="3"/>`, lx, ly, markColor)
	anchor := "middle"
	tx := lx
	if in.LowDay >= dim {
		anchor, tx = "end", lx
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" text-anchor="%s" font-size="13.5" font-weight="800" fill="%s">%s %s</text>`,
		tx, ly-13, anchor, markColor, template.HTMLEscapeString(in.EndLabel), template.HTMLEscapeString(in.LowValueStr))
	// axis day labels
	b.WriteString(`<g font-size="12.5" fill="var(--faint)" text-anchor="middle">`)
	for _, day := range axisDays(dim) {
		fmt.Fprintf(&b, `<text x="%.0f" y="222">%d</text>`, x(day), day)
	}
	b.WriteString(`</g></svg>`)
	return template.HTML(b.String()) //nolint:gosec // server-built SVG, no user HTML
}

func axisDays(dim int) []int {
	days := []int{1}
	for _, d := range []int{5, 10, 15, 20, 25} {
		if d < dim {
			days = append(days, d)
		}
	}
	return append(days, dim)
}

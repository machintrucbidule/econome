package view

import (
	"fmt"
	"html/template"
	"strings"
)

// View-models for the Net worth (Patrimoine) screens — Synthèse + Registre
// (functional/07). The handler precomputes every display string; the template
// stays logic-free (G5). Figures are the pure engine's output, formatted here.

// FSaving is one savings account in the shared rail's Épargne section (O-21).
type FSaving struct {
	ID   int64
	Name string
	Note string
	On   bool
}

// NWCard is one Synthèse metric card.
type NWCard struct {
	Label    string
	Value    string
	Help     string
	Mod      string // "" | "hl" | "good"
	CapText  string // passbook cap status (overrides the delta subtext)
	HasDelta bool
	DeltaStr string
	DeltaPos bool
	DeltaNeg bool
}

// NWLine is one row of the snapshot table. Kind ∈ support | subtotal | pea_gross
// | pea_net | total. Editable values (entered gross) are visually distinct from
// computed ones (M25).
type NWLine struct {
	Kind        string
	Label       string
	ValueStr    string
	Editable    bool
	AccountID   int64
	Period      string
	SnapshotID  int64
	HasSnapshot bool
	DelTitle    string
	DotColor    string
	Ann         string // "− 17,2 % sur gains" on the PEA net row
	Indent      bool
	RowClass    string // "" | "sub-row" | "tot-row"
	DeltaStr    string
	DeltaPos    bool
	DeltaNeg    bool
	DeltaDash   bool
}

// NetWorthView backs GET /networth (Synthèse).
type NetWorthView struct {
	Base
	Email      string
	Nav        string
	Tab        string
	Period     string
	MonthLabel string
	PrevPeriod string
	NextPeriod string
	YearLabel  string
	MonthIndex int
	PickerOpen bool

	PrevYearPeriod string
	NextYearPeriod string
	MonthCells     []MonthCell

	CurAccounts []FScope // rail current accounts (link to the budget)
	Savings     []FSaving

	RelabelStr    string
	Cards         []NWCard
	Lines         []NWLine
	CommentValue  string
	Prefilled     bool
	AutoprefillOn bool
	Empty         bool
	HasAnyHistory bool
	HasSavings    bool
	OOB           bool // render the cards fragment out-of-band (PATCH response)
}

// RRow is one Registre history-table row.
type RRow struct {
	MonthLabel string
	Period     string
	LivretsStr string
	PEAStr     string
	TotalStr   string
	DeltaStr   string
	DeltaPos   bool
	DeltaNeg   bool
	DeltaDash  bool
	Comment    string
	CommentSet bool
	Current    bool
}

// RegisterView backs GET /register (Registre).
type RegisterView struct {
	Base
	Email      string
	Nav        string
	Tab        string
	Period     string
	MonthLabel string
	PrevPeriod string
	NextPeriod string
	YearLabel  string
	MonthIndex int
	PickerOpen bool

	PrevYearPeriod string
	NextYearPeriod string
	MonthCells     []MonthCell

	CurAccounts []FScope
	Savings     []FSaving

	Range      string
	Rows       []RRow
	HasHistory bool
	ChartSVG   template.HTML
	Legend     []NWChartLegend
	FooterStr  string
	OOB        bool // render the chart fragment out-of-band (range change)
}

// NWChartLegend is one curve legend entry.
type NWChartLegend struct {
	Label string
	Color string
}

// NWChartSeries is one evolution-curve line (a support or the emphasised total).
type NWChartSeries struct {
	Color  string
	Width  float64
	Dash   bool
	Points []int64 // aligned to NWChartInput.MonthLabels (0 = absent)
}

// NWChartInput is the geometry-free input the curve renderer consumes.
type NWChartInput struct {
	MonthLabels []string
	Series      []NWChartSeries // the total first (emphasised), then supports
	EndLabel    string          // formatted total at the last point
}

// net-worth curve geometry (matches the validated register mockup viewBox).
const (
	nwVBW   = 1500.0
	nwVBH   = 280.0
	nwLeft  = 90.0
	nwRight = 1460.0
	nwTop   = 40.0
	nwBot   = 220.0
)

// RenderNetWorthChart builds the server-rendered evolution curve (M24): grid +
// k€ Y labels + one polyline per series (the total emphasised) + the end dot and
// label + sparse month X labels. Money stays integer; geometry is presentation
// float.
func RenderNetWorthChart(in NWChartInput) template.HTML {
	n := len(in.MonthLabels)
	if n == 0 || len(in.Series) == 0 {
		return ""
	}
	var maxV int64
	for _, s := range in.Series {
		for _, p := range s.Points {
			if p > maxV {
				maxV = p
			}
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	top := float64(maxV) * 1.08
	x := func(i int) float64 {
		if n == 1 {
			return nwRight
		}
		return nwLeft + (float64(i)/float64(n-1))*(nwRight-nwLeft)
	}
	y := func(v int64) float64 {
		return nwBot - (float64(v)/top)*(nwBot-nwTop)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" role="img">`, nwVBW, nwVBH)
	// grid + k€ labels at four levels
	b.WriteString(`<g stroke="var(--line)" stroke-width="1">`)
	for i := 1; i <= 4; i++ {
		gy := nwTop + (float64(i-1)/3.0)*(nwBot-nwTop)
		fmt.Fprintf(&b, `<line x1="40" y1="%.0f" x2="1480" y2="%.0f"/>`, gy, gy)
	}
	b.WriteString(`</g><g font-size="12" fill="var(--faint)" text-anchor="end">`)
	for i := 1; i <= 4; i++ {
		gy := nwTop + (float64(i-1)/3.0)*(nwBot-nwTop)
		level := top * (1.0 - float64(i-1)/3.0)
		fmt.Fprintf(&b, `<text x="34" y="%.0f">%.0f k</text>`, gy+4, level/100000.0)
	}
	b.WriteString(`</g>`)

	// series polylines
	for _, s := range in.Series {
		var poly strings.Builder
		for i, p := range s.Points {
			if i > 0 {
				poly.WriteByte(' ')
			}
			fmt.Fprintf(&poly, "%.0f,%.0f", x(i), y(p))
		}
		dash := ""
		if s.Dash {
			dash = ` stroke-dasharray="5 3"`
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%.1f" stroke-linejoin="round" stroke-linecap="round"%s/>`,
			poly.String(), s.Color, s.Width, dash)
	}
	// end dot + label on the total (first series)
	total := in.Series[0]
	if len(total.Points) > 0 {
		lx, ly := x(n-1), y(total.Points[len(total.Points)-1])
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="6" fill="%s"/>`, lx, ly, total.Color)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" text-anchor="end" font-size="13.5" font-weight="800" fill="%s">%s</text>`,
			lx-8, ly-10, total.Color, template.HTMLEscapeString(in.EndLabel))
	}
	// sparse X month labels (up to ~6)
	b.WriteString(`<g font-size="12" fill="var(--faint)" text-anchor="middle">`)
	for _, i := range nwAxisTicks(n) {
		fmt.Fprintf(&b, `<text x="%.0f" y="244">%s</text>`, x(i), template.HTMLEscapeString(in.MonthLabels[i]))
	}
	b.WriteString(`</g></svg>`)
	return template.HTML(b.String()) //nolint:gosec // server-built SVG, no user HTML
}

// nwAxisTicks picks up to six evenly-spaced label indices over n points.
func nwAxisTicks(n int) []int {
	if n <= 1 {
		return []int{0}
	}
	maxTicks := 6
	if n < maxTicks {
		maxTicks = n
	}
	seen := map[int]bool{}
	var out []int
	for k := 0; k < maxTicks; k++ {
		i := k * (n - 1) / (maxTicks - 1)
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}

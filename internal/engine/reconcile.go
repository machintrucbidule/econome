package engine

import "econome/internal/domain"

// Reconciliation is a pure decision function over a candidate set — the same
// function the manual journal path uses today and the DSP2 import will call
// tomorrow (technical/09 §3, functional/04 §7). It only decides; the service
// performs the DB write (edit-in-place, no duplicate, L6).

// Sign is the direction of a movement: +1 for an inflow (income / transfer-in),
// −1 for an outflow (expense / transfer-out).
type Sign int

// Movement is an observed (cleared) movement to reconcile.
type Movement struct {
	Account int64
	Sign    Sign
	Amount  int64 // positive magnitude (minor units)
	Date    domain.Date
}

// Candidate is an awaited row that a movement might match.
type Candidate struct {
	TxnID        int64
	Account      int64
	Sign         Sign
	Amount       int64 // positive magnitude
	ExpectedDate domain.Date
	Period       string
}

// Tolerance bounds a match: amount within Amount (0 = exact, for manual entry),
// date within DateWindowDays of the candidate's expected date.
type Tolerance struct {
	Amount         int64
	DateWindowDays int
}

// DecisionKind enumerates the reconciliation outcomes.
type DecisionKind int

// Decision kinds.
const (
	CreateNew DecisionKind = iota
	ReconcileInPlace
	Ambiguous
)

// Decision is the reconciliation outcome (I-014): a small tagged struct rather
// than an interface, so callers switch exhaustively and avoid type assertions.
type Decision struct {
	Kind         DecisionKind
	TxnID        int64   // set for ReconcileInPlace
	AmbiguousIDs []int64 // set for Ambiguous
}

// Reconcile matches a cleared movement against awaited candidates on the same
// account and sign, amount within tolerance, and date within the window:
// exactly one match ⇒ ReconcileInPlace; zero ⇒ CreateNew; several ⇒ Ambiguous
// (no silent guess).
func Reconcile(m Movement, candidates []Candidate, tol Tolerance) Decision {
	var matches []int64
	for _, c := range candidates {
		if matchesKeys(m, c, tol) {
			matches = append(matches, c.TxnID)
		}
	}
	return decide(matches)
}

// PairTransfer matches one observed transfer leg against candidate opposite legs
// for internal-transfer auto-pairing (functional/04 §7, rules §10): equal amount,
// opposite sign, close dates, a different (internal) account. Same outcomes as
// Reconcile.
func PairTransfer(leg Movement, candidates []Candidate, tol Tolerance) Decision {
	var matches []int64
	for _, c := range candidates {
		if matchesPair(leg, c, tol) {
			matches = append(matches, c.TxnID)
		}
	}
	return decide(matches)
}

func decide(matches []int64) Decision {
	switch len(matches) {
	case 0:
		return Decision{Kind: CreateNew}
	case 1:
		return Decision{Kind: ReconcileInPlace, TxnID: matches[0]}
	default:
		return Decision{Kind: Ambiguous, AmbiguousIDs: matches}
	}
}

func matchesKeys(m Movement, c Candidate, tol Tolerance) bool {
	return m.Account == c.Account &&
		m.Sign == c.Sign &&
		absInt64(m.Amount-c.Amount) <= tol.Amount &&
		dateDiffDays(m.Date, c.ExpectedDate) <= tol.DateWindowDays
}

func matchesPair(leg Movement, c Candidate, tol Tolerance) bool {
	return leg.Account != c.Account && // both internal, different accounts
		leg.Sign == -c.Sign && // opposite legs
		absInt64(leg.Amount-c.Amount) <= tol.Amount &&
		dateDiffDays(leg.Date, c.ExpectedDate) <= tol.DateWindowDays
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// dateDiffDays returns the absolute difference in days between two dates, using
// the proleptic-Gregorian day number (Howard Hinnant's days_from_civil, O(1)).
func dateDiffDays(a, b domain.Date) int {
	diff := daysFromCivil(a) - daysFromCivil(b)
	if diff < 0 {
		diff = -diff
	}
	return diff
}

func daysFromCivil(d domain.Date) int {
	y := d.Year
	if d.Month <= 2 {
		y--
	}
	era := y / 400
	yoe := y - era*400
	mp := d.Month + 9
	if d.Month > 2 {
		mp = d.Month - 3
	}
	doy := (153*mp+2)/5 + d.Day - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

package services

import (
	"context"
	"errors"
	"time"

	"econome/internal/auth"
	"econome/internal/domain"
	"econome/internal/repo"
)

// Session lifetimes (functional/01 §1.3, technical/05 §2).
const (
	shortSessionLifetime    = 24 * time.Hour
	rememberSessionLifetime = 365 * 24 * time.Hour
)

// ErrInvalidCredentials is the single generic login failure (never reveals which
// field was wrong or whether the email exists — technical/05 §6).
var ErrInvalidCredentials = errors.New("services: invalid credentials")

// decoyHash is a fixed Argon2id hash verified against the login password when no
// user matches the email, so a not-found login costs the same time as a real
// verify (anti-enumeration, technical/05 §6). The decoy password is not a secret.
var decoyHash = mustHash("econome-timing-equaliser-decoy")

func mustHash(pw string) string {
	h, err := auth.HashPassword(pw)
	if err != nil {
		panic("services: decoy hash: " + err.Error())
	}
	return h
}

// LockedError signals that the account or IP is temporarily throttled.
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string { return "services: locked" }

// RetrySeconds rounds the remaining lock up to whole seconds for display.
func (e *LockedError) RetrySeconds() int {
	s := int(e.RetryAfter / time.Second)
	if e.RetryAfter%time.Second > 0 {
		s++
	}
	if s < 1 {
		s = 1
	}
	return s
}

// Service owns the application use-cases: it loads inputs via the repositories,
// applies validation, the lockout policy, and the locked-month guard, writes
// inside transactions, and returns results. It holds no HTTP knowledge.
type Service struct {
	users        repo.UserRepo
	sessions     repo.SessionRepo
	settings     repo.SettingsRepo
	accounts     repo.AccountRepo
	categories   repo.CategoryRepo
	envelopes    repo.EnvelopeRepo
	allocations  repo.AllocationRepo
	transactions repo.TransactionRepo
	snapshots    repo.SnapshotRepo
	periods      repo.PeriodRepo
	periodEvents repo.PeriodEventRepo
	tx           repo.Txer
	secret       []byte
	throttle     *auth.Throttle
	now          func() time.Time
}

// Deps bundles the repositories + transaction seam + session secret the service
// needs. Both *repo.Store (SQLite) and *repotest.Store (fakes) supply these
// interface-typed fields, so a named struct keeps the wiring readable as the
// service grows across increments rather than a long positional argument list.
type Deps struct {
	Users        repo.UserRepo
	Sessions     repo.SessionRepo
	Settings     repo.SettingsRepo
	Accounts     repo.AccountRepo
	Categories   repo.CategoryRepo
	Envelopes    repo.EnvelopeRepo
	Allocations  repo.AllocationRepo
	Transactions repo.TransactionRepo
	Snapshots    repo.SnapshotRepo
	Periods      repo.PeriodRepo
	PeriodEvents repo.PeriodEventRepo
	Tx           repo.Txer
	Secret       []byte
}

// New builds a Service from its dependencies. Tests inject fakes for the same
// interfaces.
func New(d Deps) *Service {
	return &Service{
		users:        d.Users,
		sessions:     d.Sessions,
		settings:     d.Settings,
		accounts:     d.Accounts,
		categories:   d.Categories,
		envelopes:    d.Envelopes,
		allocations:  d.Allocations,
		transactions: d.Transactions,
		snapshots:    d.Snapshots,
		periods:      d.Periods,
		periodEvents: d.PeriodEvents,
		tx:           d.Tx,
		secret:       d.Secret,
		throttle:     auth.NewThrottle(20, time.Minute),
		now:          time.Now,
	}
}

// AuthResult is the outcome of a successful setup or login.
type AuthResult struct {
	Token     string
	Kind      domain.SessionKind
	ExpiresAt time.Time
	User      *domain.User
}

// ZeroUsers reports whether the instance has no users yet (the setup guard).
func (s *Service) ZeroUsers(ctx context.Context) (bool, error) {
	n, err := s.users.CountUsers(ctx, s.tx.DB())
	return n == 0, err
}

// SetupInput is the owner-creation form.
type SetupInput struct {
	Email, Password, PasswordConfirm string
	Language, Currency               string
}

// Setup creates the owner account + its settings row atomically and opens a
// session. It validates the input (typed 422) and refuses once any user exists
// (functional/01 §2).
func (s *Service) Setup(ctx context.Context, in SetupInput) (*AuthResult, error) {
	if err := s.validateSetup(in); err != nil {
		return nil, err
	}
	lang := normaliseLang(in.Language)
	currency := normaliseCurrency(in.Currency)

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	user := &domain.User{
		Email: in.Email, PasswordHash: hash, IsAdmin: true, Status: domain.StatusActive,
		Language: lang, Currency: currency, CreatedAt: now, UpdatedAt: now,
	}
	settings := defaultSettings(lang, currency, now)

	err = s.tx.WithTx(ctx, func(q repo.DBTX) error {
		n, err := s.users.CountUsers(ctx, q)
		if err != nil {
			return err
		}
		if n > 0 {
			return domain.ErrConflict // wizard is unreachable once an owner exists
		}
		id, err := s.users.Create(ctx, q, user)
		if err != nil {
			return err
		}
		user.ID = id
		settings.UserID = id
		return s.settings.Create(ctx, q, settings)
	})
	if err != nil {
		return nil, err
	}

	return s.issueSession(ctx, user, false)
}

func (s *Service) validateSetup(in SetupInput) error {
	v := &domain.ValidationError{}
	mergeValidation(v, domain.ValidateEmail(in.Email))
	mergeValidation(v, domain.ValidatePassword(in.Password))
	if in.Password != in.PasswordConfirm {
		v.Add("password_confirm", domain.MsgPasswordMismatch)
	}
	return v.OrNil()
}

// LoginInput is the login form plus request metadata.
type LoginInput struct {
	Email, Password string
	Remember        bool
	IP, UserAgent   string
}

// Login verifies the credentials and opens a session, applying the per-IP
// throttle and the per-account progressive lockout, with a single generic error
// on any failure (technical/05 §6).
func (s *Service) Login(ctx context.Context, in LoginInput) (*AuthResult, error) {
	now := s.now().UTC()
	if !s.throttle.Allow(in.IP, now) {
		return nil, &LockedError{RetryAfter: time.Minute}
	}

	user, err := s.users.GetByEmail(ctx, s.tx.DB(), in.Email)
	if errors.Is(err, domain.ErrNotFound) {
		// Equalise timing against a known-email path so the response time does
		// not reveal whether the address exists (technical/05 §6 — no user
		// enumeration). Run a dummy Argon2id verify with the default parameters.
		_, _, _ = auth.VerifyPassword(decoyHash, in.Password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if d := auth.RemainingLock(user.LockedUntil, now); d > 0 {
		return nil, &LockedError{RetryAfter: d}
	}

	ok, needsRehash, err := auth.VerifyPassword(user.PasswordHash, in.Password)
	if err != nil {
		return nil, err
	}
	if !ok {
		s.recordFailure(ctx, user, now)
		return nil, ErrInvalidCredentials
	}

	// Success: reset the counter, transparently re-hash if parameters changed.
	user.FailedLoginCount = 0
	user.LastFailedLoginAt = nil
	user.LockedUntil = nil
	user.UpdatedAt = now
	if err := s.users.UpdateLoginState(ctx, s.tx.DB(), user); err != nil {
		return nil, err
	}
	if needsRehash {
		if nh, e := auth.HashPassword(in.Password); e == nil {
			_ = s.users.UpdatePasswordHash(ctx, s.tx.DB(), user.ID, nh)
		}
	}
	return s.issueSession(ctx, user, in.Remember)
}

func (s *Service) recordFailure(ctx context.Context, user *domain.User, now time.Time) {
	user.FailedLoginCount++
	user.LastFailedLoginAt = &now
	if bo := auth.BackoffFor(user.FailedLoginCount); bo > 0 {
		lu := now.Add(bo)
		user.LockedUntil = &lu
	}
	user.UpdatedAt = now
	_ = s.users.UpdateLoginState(ctx, s.tx.DB(), user)
}

// Logout revokes the session for the given raw cookie token (idempotent).
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	sess, err := s.sessions.GetByTokenHash(ctx, s.tx.DB(), auth.HashToken(rawToken))
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.sessions.Delete(ctx, s.tx.DB(), sess.ID)
}

// ResolveSession loads the user + session for a raw cookie token, expiring stale
// or deactivated sessions and sliding the expiry on activity. It returns
// domain.ErrNotFound for any anonymous/invalid case.
func (s *Service) ResolveSession(ctx context.Context, rawToken string) (*domain.User, *domain.Session, error) {
	sess, err := s.sessions.GetByTokenHash(ctx, s.tx.DB(), auth.HashToken(rawToken))
	if err != nil {
		return nil, nil, err
	}
	now := s.now().UTC()
	if now.After(sess.ExpiresAt) {
		_ = s.sessions.Delete(ctx, s.tx.DB(), sess.ID)
		return nil, nil, domain.ErrNotFound
	}
	user, err := s.users.GetByID(ctx, s.tx.DB(), sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	if user.Status != domain.StatusActive {
		_ = s.sessions.DeleteByUser(ctx, s.tx.DB(), user.ID)
		return nil, nil, domain.ErrNotFound
	}
	newExp := now.Add(lifetimeFor(sess.Kind))
	_ = s.sessions.Touch(ctx, s.tx.DB(), sess.ID, now, newExp)
	sess.LastSeenAt = now
	sess.ExpiresAt = newExp
	return user, sess, nil
}

// Settings returns the user's settings row.
func (s *Service) Settings(ctx context.Context, userID int64) (*domain.Settings, error) {
	return s.settings.Get(ctx, s.tx.DB(), userID)
}

// Secret exposes the session secret (used by the CSRF middleware).
func (s *Service) Secret() []byte { return s.secret }

// Health performs a trivial query to confirm the database is reachable
// (backs GET /healthz).
func (s *Service) Health(ctx context.Context) error {
	_, err := s.users.CountUsers(ctx, s.tx.DB())
	return err
}

func (s *Service) issueSession(ctx context.Context, user *domain.User, remember bool) (*AuthResult, error) {
	raw, err := auth.GenerateSessionToken()
	if err != nil {
		return nil, err
	}
	kind := domain.SessionShort
	if remember {
		kind = domain.SessionRemember
	}
	now := s.now().UTC()
	sess := &domain.Session{
		UserID: user.ID, TokenHash: auth.HashToken(raw), Kind: kind,
		ExpiresAt: now.Add(lifetimeFor(kind)), CreatedAt: now, LastSeenAt: now,
	}
	if _, err := s.sessions.Create(ctx, s.tx.DB(), sess); err != nil {
		return nil, err
	}
	return &AuthResult{Token: raw, Kind: kind, ExpiresAt: sess.ExpiresAt, User: user}, nil
}

func lifetimeFor(kind domain.SessionKind) time.Duration {
	switch kind {
	case domain.SessionRemember:
		return rememberSessionLifetime
	case domain.SessionShort:
		return shortSessionLifetime
	default:
		return shortSessionLifetime
	}
}

func defaultSettings(lang domain.Language, currency string, now time.Time) *domain.Settings {
	return &domain.Settings{
		PEASocialChargeRate: 1720,
		NearCapThreshold:    9000,
		SecuredSavingsBasis: domain.BasisAllPlanned,
		Theme:               domain.ThemeLight,
		Language:            lang,
		Currency:            currency,
		UpdatedAt:           now,
	}
}

func mergeValidation(dst *domain.ValidationError, err error) {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		dst.Fields = append(dst.Fields, ve.Fields...)
	}
}

func normaliseLang(s string) domain.Language {
	if domain.Language(s) == domain.LangEN {
		return domain.LangEN
	}
	return domain.LangFR
}

func normaliseCurrency(s string) string {
	if s == "" {
		return "EUR"
	}
	return s
}

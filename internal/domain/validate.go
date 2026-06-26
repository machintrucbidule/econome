package domain

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PasswordMinLength is the §9 minimum (decision A8).
const PasswordMinLength = 12

// Message keys (resolved to localised text by the view layer).
const (
	MsgEmailRequired    = "validation.email.required"
	MsgEmailInvalid     = "validation.email.invalid"
	MsgPasswordTooShort = "validation.password.too_short"
	MsgPasswordClasses  = "validation.password.classes"
	MsgPasswordMismatch = "validation.password.mismatch"
)

// PasswordCriteria reports which password-policy rules a candidate satisfies.
// It backs both server validation and the live checklist shown on the setup /
// password screens (functional/01 §9).
type PasswordCriteria struct {
	MinLength bool
	Lower     bool
	Upper     bool
	Digit     bool
	Symbol    bool
}

// AllMet reports whether every criterion passes.
func (c PasswordCriteria) AllMet() bool {
	return c.MinLength && c.Lower && c.Upper && c.Digit && c.Symbol
}

// CheckPassword classifies a candidate password against the §9 policy. A symbol
// is any rune that is not a letter, digit, or whitespace.
func CheckPassword(pw string) PasswordCriteria {
	var c PasswordCriteria
	c.MinLength = utf8.RuneCountInString(pw) >= PasswordMinLength
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			c.Lower = true
		case unicode.IsUpper(r):
			c.Upper = true
		case unicode.IsDigit(r):
			c.Digit = true
		case !unicode.IsLetter(r) && !unicode.IsSpace(r):
			c.Symbol = true
		}
	}
	return c
}

// ValidateEmail checks that the address is non-empty and well-formed. It
// returns nil on success or a *ValidationError on the "email" field.
func ValidateEmail(email string) error {
	v := &ValidationError{}
	if strings.TrimSpace(email) == "" {
		v.Add("email", MsgEmailRequired)
		return v.OrNil()
	}
	if _, err := mail.ParseAddress(email); err != nil {
		v.Add("email", MsgEmailInvalid)
	}
	return v.OrNil()
}

// ValidatePassword enforces the §9 policy (length + four character classes). It
// returns nil on success or a *ValidationError on the "password" field.
func ValidatePassword(pw string) error {
	c := CheckPassword(pw)
	if c.AllMet() {
		return nil
	}
	v := &ValidationError{}
	if !c.MinLength {
		v.Add("password", MsgPasswordTooShort)
	}
	if !c.Lower || !c.Upper || !c.Digit || !c.Symbol {
		v.Add("password", MsgPasswordClasses)
	}
	return v.OrNil()
}

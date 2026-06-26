package domain

import "testing"

func TestCheckPassword(t *testing.T) {
	cases := []struct {
		pw  string
		all bool
	}{
		{"Abcdef1!ghij", true},     // 12 chars, all classes
		{"Abcdef1!", false},        // too short (8)
		{"abcdefghij1!", false},    // no upper
		{"ABCDEFGHIJ1!", false},    // no lower
		{"Abcdefghij!!", false},    // no digit
		{"Abcdefghij12", false},    // no symbol
		{"Tr0ub4dour&3xtra", true}, // long, all classes
	}
	for _, c := range cases {
		if got := CheckPassword(c.pw).AllMet(); got != c.all {
			t.Errorf("CheckPassword(%q).AllMet() = %v, want %v", c.pw, got, c.all)
		}
		err := ValidatePassword(c.pw)
		if c.all && err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil", c.pw, err)
		}
		if !c.all && err == nil {
			t.Errorf("ValidatePassword(%q) = nil, want error", c.pw)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	good := []string{"a@b.co", "ivan.calmels@example.org", "x+tag@sub.domain.fr"}
	bad := []string{"", "   ", "notanemail", "@nodomain", "a@"}
	for _, e := range good {
		if err := ValidateEmail(e); err != nil {
			t.Errorf("ValidateEmail(%q) = %v, want nil", e, err)
		}
	}
	for _, e := range bad {
		if err := ValidateEmail(e); err == nil {
			t.Errorf("ValidateEmail(%q) = nil, want error", e)
		}
	}
}

func TestEnumValid(t *testing.T) {
	if !StatusActive.Valid() || StatusDeactivated.Valid() != true || Status("x").Valid() {
		t.Error("Status.Valid wrong")
	}
	if !LangFR.Valid() || Language("de").Valid() {
		t.Error("Language.Valid wrong")
	}
	if !SessionRemember.Valid() || SessionKind("x").Valid() {
		t.Error("SessionKind.Valid wrong")
	}
	if !BasisFixedOnly.Valid() || SecuredSavingsBasis("x").Valid() {
		t.Error("SecuredSavingsBasis.Valid wrong")
	}
	if !ThemeDark.Valid() || Theme("x").Valid() {
		t.Error("Theme.Valid wrong")
	}
}

func TestValidationErrorAggregate(t *testing.T) {
	v := &ValidationError{}
	if v.OrNil() != nil {
		t.Fatal("empty ValidationError should be nil via OrNil")
	}
	v.Add("email", MsgEmailRequired)
	v.Add("password", MsgPasswordTooShort)
	if !v.HasErrors() || len(v.Fields) != 2 {
		t.Fatal("expected 2 field errors")
	}
	if v.OrNil() == nil {
		t.Fatal("non-empty ValidationError should be non-nil via OrNil")
	}
	if v.Error() == "" {
		t.Fatal("Error() should be non-empty")
	}
}

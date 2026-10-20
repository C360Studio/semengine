package rulepack

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

func TestValidateID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		packID  string
		wantErr string
	}{
		{name: "minimum", packID: "a"},
		{name: "full alphabet", packID: "Pack-v1_test=value-1"},
		{name: "maximum", packID: strings.Repeat("p", MaxPackIDBytes)},
		{name: "empty", wantErr: "pack_id is required"},
		{name: "oversized", packID: strings.Repeat("p", MaxPackIDBytes+1), wantErr: "maximum is 246"},
		{name: "invalid alphabet", packID: "pack:one", wantErr: "invalid pack_id"},
		{name: "separator rejected", packID: "pack.one", wantErr: "one literal KV token"},
		{name: "slash rejected", packID: "pack/one", wantErr: "invalid pack_id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateID(test.packID)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateID(%q): %v", test.packID, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateID(%q) error = %v, want substring %q", test.packID, err, test.wantErr)
			}
		})
	}
}

// TestValidateIDEveryByte places each of the 256 byte values between two
// accepted bytes. Every refusal rule is per byte and position-free, so this
// table decides every alphabet case. The oracle is the rule-pack-identity
// spec's pattern, written here rather than read from PackIDCharset.
func TestValidateIDEveryByte(t *testing.T) {
	t.Parallel()

	oracle := regexp.MustCompile(`^[A-Za-z0-9_=-]+$`)
	accepted := 0
	for b := 0; b < 256; b++ {
		packID := "a" + string([]byte{byte(b)}) + "a"
		want := oracle.MatchString(packID)
		err := ValidateID(packID)
		if want && err != nil {
			t.Errorf("ValidateID(%q) byte 0x%02x: %v, want accepted", packID, b, err)
		}
		if !want && err == nil {
			t.Errorf("ValidateID(%q) byte 0x%02x accepted, want refused", packID, b)
		}
		if err == nil {
			accepted++
		}
	}
	// 26 upper, 26 lower, 10 digits, and _ = -.
	if accepted != 65 {
		t.Fatalf("accepted %d byte values, want 65", accepted)
	}
}

func TestValidateIDBounds(t *testing.T) {
	t.Parallel()

	if err := ValidateID(strings.Repeat("p", 246)); err != nil {
		t.Fatalf("246-byte ID: %v, want accepted", err)
	}
	if err := ValidateID(strings.Repeat("p", 247)); err == nil {
		t.Fatal("247-byte ID accepted, want refused")
	}
}

// TestValidateIDKVTokenRefusal checks that a value that is not one literal KV
// token carries natsclient's classified error: natsclient.ValidateKVLiteralToken
// returns a *errs.ClassifiedError (built by natsclient's newKVContractError)
// whose Code is natsclient.ErrorCodeKVTokenInvalid. The literal below is the
// spec's.
func TestValidateIDKVTokenRefusal(t *testing.T) {
	t.Parallel()

	err := ValidateID("pack.one")
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) {
		t.Fatalf("ValidateID(%q) error = %v (%T), want a wrapped *errs.ClassifiedError", "pack.one", err, err)
	}
	if classified.Code != "kv_token_invalid" {
		t.Fatalf("code = %q, want %q", classified.Code, "kv_token_invalid")
	}
}

// TestValidateIDRefusalPrecedence checks the spec's check order: an empty ID
// and an overlength ID are refused by the required and length checks before
// the KV-token check runs, so neither carries the classified kv_token_invalid
// error even though neither is a valid literal KV token.
func TestValidateIDRefusalPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		packID  string
		wantErr string
	}{
		{name: "empty", packID: "", wantErr: "pack_id is required"},
		{name: "overlength with dot", packID: strings.Repeat("a", MaxPackIDBytes) + ".", wantErr: "maximum is 246"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateID(test.packID)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateID(%q) error = %v, want substring %q", test.packID, err, test.wantErr)
			}
			var classified *errs.ClassifiedError
			if errors.As(err, &classified) {
				t.Fatalf("ValidateID(%q) error = %v, want no classified error before the token check", test.packID, err)
			}
		})
	}
}

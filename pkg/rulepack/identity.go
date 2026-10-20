// Package rulepack owns the identity contract for composed rule processors:
// the grammar and byte bound of a rule pack's pack_id. It depends only on the
// public NATS KV-key contract, so services can check a pack ID without
// importing the rule implementation.
package rulepack

import (
	"fmt"
	"regexp"

	"github.com/c360studio/semengine/natsclient"
)

const (
	// PackIDCharset is the exact ASCII alphabet accepted for rule-pack IDs.
	PackIDCharset = "[A-Za-z0-9_=-]"
	// MaxPackIDBytes is the largest rule-pack ID, in bytes, that ValidateID
	// accepts (rule-pack-identity spec).
	MaxPackIDBytes = 246
)

// packIDPattern is built from PackIDCharset so the validator and the
// alphabet the constant publishes cannot disagree.
var packIDPattern = regexp.MustCompile("^" + PackIDCharset + "+$")

// ValidateID validates the public, stable rule-pack identity contract.
func ValidateID(packID string) error {
	if packID == "" {
		return fmt.Errorf("rule config: pack_id is required")
	}
	if len(packID) > MaxPackIDBytes {
		return fmt.Errorf("rule config: pack_id is %d bytes; maximum is %d", len(packID), MaxPackIDBytes)
	}
	if err := natsclient.ValidateKVLiteralToken(packID); err != nil {
		return fmt.Errorf(
			"rule config: invalid pack_id %q — must use one literal KV token from %s: %w",
			packID, PackIDCharset, err,
		)
	}
	if !packIDPattern.MatchString(packID) {
		return fmt.Errorf("rule config: invalid pack_id %q — must use only %s", packID, PackIDCharset)
	}
	return nil
}

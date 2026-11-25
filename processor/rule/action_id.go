package rule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// defaultActionMaxIterations is the firing cap of an action whose
// `max_iterations` is absent. Structured-output retries are common, so three
// gives one corrective shot plus a margin and makes persistent drift loud.
// An author who wants no cap sets `"max_iterations": 0`.
const defaultActionMaxIterations = 3

// capKeyLen is the length in hex characters of a hashed firing-cap key: 8
// bytes of SHA-256, short enough to read in logs and stored state, and
// collisions are improbable across any rule set people write.
const capKeyLen = 16

// capKey is the key the action's firing count is kept under: the action's
// `id` when set, otherwise a hash of the rule ID, the type and the whole body
// in canonical form. Every key in the body counts, `max_iterations` and
// `when` included, so any edit starts a fresh count, while storing the rule
// or reformatting its file keeps the count.
func capKey(ruleID string, a Action) (string, error) {
	if a.ID != "" {
		return a.ID, nil
	}
	body, err := canonicalBody(a.Body)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	// Each part is length-prefixed, so no choice of rule ID or type can make
	// two different triples hash the same input.
	for _, part := range []string{ruleID, a.Type, string(body)} {
		fmt.Fprintf(h, "%d:%s", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))[:capKeyLen], nil
}

// canonicalBody writes body with numbers as written, object keys sorted, no
// spacing and no HTML escaping, so that compacting, escaping or reformatting
// the same object gives the same bytes.
func canonicalBody(body json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("action body: %w", err)
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("action body: %w", err)
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}

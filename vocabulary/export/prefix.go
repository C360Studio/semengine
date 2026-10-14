package export

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"

	"github.com/c360studio/semengine/vocabulary"
)

// defaultPrefixes maps short prefix names to their namespace IRIs.
// Only prefixes that are actually used in output are emitted.
//
// The baseline set ships with the package; optional downstream
// vocabulary packages can extend it from init() via Register. Read
// access is mutex-guarded because
// downstream code may register additional namespaces from init in
// any package import order, and exporters snapshot the map at
// newPrefixMap time.
var (
	defaultPrefixesMu sync.RWMutex
	defaultPrefixes   = map[string]string{
		"rdf":    "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
		"owl":    "http://www.w3.org/2002/07/owl#",
		"skos":   "http://www.w3.org/2004/02/skos/core#",
		"rdfs":   "http://www.w3.org/2000/01/rdf-schema#",
		"dc":     "http://purl.org/dc/terms/",
		"schema": "https://schema.org/",
		"foaf":   "http://xmlns.com/foaf/0.1/",
		"prov":   "http://www.w3.org/ns/prov#",
		"ssn":    "http://www.w3.org/ns/ssn/",
		"sosa":   "http://www.w3.org/ns/sosa/",
		"xsd":    "http://www.w3.org/2001/XMLSchema#",
		"ss":     vocabulary.SemEngineBase + "/",
	}
)

// Register adds a prefix → namespace binding to the export prefix
// table. It is idempotent: registering the same prefix with the same
// namespace twice is a no-op. Registering the same prefix with a
// different namespace returns an error so collisions surface loudly
// rather than silently shadowing.
//
// Intended use is from a package init() in a downstream vocabulary
// package, so registrations land before any exporter runs. The function
// is safe to call at runtime as well, but mid-export registrations will
// only affect prefix maps constructed after the call returns.
func Register(prefix, namespace string) error {
	return RegisterPrefixes([]vocabulary.PrefixSpec{{
		Prefix:    prefix,
		Namespace: namespace,
	}})
}

// RegisterPrefixes atomically validates and registers a batch of prefix →
// namespace bindings. Every entry is validated against vocabulary.PrefixSpec.Validate
// (Turtle PN_PREFIX grammar and non-empty namespace IRI), intra-batch
// consistency, and existing bindings before any binding is committed. If any
// entry is invalid or conflicts with an existing binding, the prefix table is
// left unchanged.
func RegisterPrefixes(prefixes []vocabulary.PrefixSpec) error {
	defaultPrefixesMu.Lock()
	defer defaultPrefixesMu.Unlock()

	staged := make(map[string]string, len(prefixes))
	for i, pfx := range prefixes {
		if err := pfx.Validate(); err != nil {
			return fmt.Errorf("vocabulary/export: prefixes[%d]: %w", i, err)
		}
		if existing, ok := defaultPrefixes[pfx.Prefix]; ok && existing != pfx.Namespace {
			return fmt.Errorf("vocabulary/export: prefix %q already bound to %q, cannot rebind to %q", pfx.Prefix, existing, pfx.Namespace)
		}
		if existing, ok := staged[pfx.Prefix]; ok && existing != pfx.Namespace {
			return fmt.Errorf("vocabulary/export: prefix %q bound to conflicting namespaces %q and %q in batch", pfx.Prefix, existing, pfx.Namespace)
		}
		staged[pfx.Prefix] = pfx.Namespace
	}
	for prefix, ns := range staged {
		defaultPrefixes[prefix] = ns
	}
	return nil
}

// snapshotDefaults returns a copy of the current prefix table under
// the read lock. Callers iterate the returned map without holding
// any lock.
func snapshotDefaults() map[string]string {
	defaultPrefixesMu.RLock()
	defer defaultPrefixesMu.RUnlock()
	out := make(map[string]string, len(defaultPrefixes))
	maps.Copy(out, defaultPrefixes)
	return out
}

// prefixMap tracks namespaces and which prefixes are actually used.
type prefixMap struct {
	prefixes map[string]string // prefix -> namespace
	used     map[string]bool   // prefix -> whether it appeared in output
}

// newPrefixMap creates a prefix map initialized with defaults, optionally
// overriding the ss: prefix if a custom base IRI is provided.
func newPrefixMap(base string) *prefixMap {
	defaults := snapshotDefaults()
	pm := &prefixMap{
		prefixes: defaults,
		used:     make(map[string]bool, len(defaults)),
	}
	// Override ss: if custom base
	if base != "" && base != vocabulary.SemEngineBase {
		pm.prefixes["ss"] = ensureTrailingSlash(base)
	}
	return pm
}

// compact attempts to shorten a full IRI using known prefixes.
// Returns the compacted form (e.g., "owl:sameAs") and true if successful,
// or the original IRI and false if no prefix matches.
func (pm *prefixMap) compact(iri string) (string, bool) {
	// Try each prefix, preferring the longest namespace match; break equal-length
	// namespace ties by lexicographically smallest prefix so namespace aliases
	// select the same prefix across scanPrefixes and rendering.
	var bestPrefix string
	var bestNS string
	for prefix, ns := range pm.prefixes {
		if !strings.HasPrefix(iri, ns) {
			continue
		}
		if len(ns) > len(bestNS) || (len(ns) == len(bestNS) && (bestPrefix == "" || prefix < bestPrefix)) {
			bestPrefix = prefix
			bestNS = ns
		}
	}
	if bestNS == "" {
		return iri, false
	}
	local := iri[len(bestNS):]
	// Only compact if the local part is a valid unescaped Turtle PN_LOCAL (#250).
	if !isValidPNLocal(local) {
		return iri, false
	}
	pm.used[bestPrefix] = true
	return bestPrefix + ":" + local, true
}

func isValidPNLocal(local string) bool {
	if local == "" {
		return false
	}
	var lastRune rune
	for i, r := range local {
		if i == 0 {
			if !isPNLocalStart(r) {
				return false
			}
		} else if r != '.' && !isPNLocalChar(r) {
			return false
		}
		lastRune = r
	}
	return lastRune != '.'
}

func isPNLocalStart(r rune) bool {
	return isPNLocalBase(r) || r == '_' || (r >= '0' && r <= '9')
}

func isPNLocalChar(r rune) bool {
	return isPNLocalStart(r) ||
		r == '-' ||
		r == 0x00B7 ||
		(r >= 0x0300 && r <= 0x036F) ||
		(r >= 0x203F && r <= 0x2040)
}

func isPNLocalBase(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 0x00C0 && r <= 0x00D6) ||
		(r >= 0x00D8 && r <= 0x00F6) ||
		(r >= 0x00F8 && r <= 0x02FF) ||
		(r >= 0x0370 && r <= 0x037D) ||
		(r >= 0x037F && r <= 0x1FFF) ||
		(r >= 0x200C && r <= 0x200D) ||
		(r >= 0x2070 && r <= 0x218F) ||
		(r >= 0x2C00 && r <= 0x2FEF) ||
		(r >= 0x3001 && r <= 0xD7FF) ||
		(r >= 0xF900 && r <= 0xFDCF) ||
		(r >= 0xFDF0 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0xEFFFF)
}

// markUsed records a prefix as used without compacting.
func (pm *prefixMap) markUsed(prefix string) {
	pm.used[prefix] = true
}

// usedPrefixes returns the prefixes that were actually used, sorted alphabetically.
func (pm *prefixMap) usedPrefixes() []string {
	result := make([]string, 0, len(pm.used))
	for p := range pm.used {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

// namespaceFor returns the namespace IRI for a given prefix.
func (pm *prefixMap) namespaceFor(prefix string) string {
	return pm.prefixes[prefix]
}

// usedPrefixMap returns a map of used prefix -> namespace.
func (pm *prefixMap) usedPrefixMap() map[string]string {
	result := make(map[string]string, len(pm.used))
	for p := range pm.used {
		result[p] = pm.prefixes[p]
	}
	return result
}

// jsonldContext returns the JSON-LD @context map for all used prefixes.
// Per W3C JSON-LD 1.1 Processing Algorithms (Create Term Definition, step 14.2.5),
// a simple string term definition is only treated as a prefix when its IRI ends
// with an RFC 3986 gen-delim character (':', '/', '?', '#', '[', ']', '@').
// Namespaces ending in other characters (such as OBO/BFO's '_') require an
// expanded term definition with "@prefix": true so compact IRIs expand back to
// their original IRI (#249).
func (pm *prefixMap) jsonldContext() map[string]any {
	result := make(map[string]any, len(pm.used))
	for p := range pm.used {
		ns := pm.prefixes[p]
		if hasJSONLDGenDelimSuffix(ns) {
			result[p] = ns
		} else {
			result[p] = map[string]any{
				"@id":     ns,
				"@prefix": true,
			}
		}
	}
	return result
}

func hasJSONLDGenDelimSuffix(ns string) bool {
	if ns == "" {
		return false
	}
	switch ns[len(ns)-1] {
	case ':', '/', '?', '#', '[', ']', '@':
		return true
	default:
		return false
	}
}

func ensureTrailingSlash(s string) string {
	if s == "" {
		return s
	}
	if s[len(s)-1] == '/' || s[len(s)-1] == '#' {
		return s
	}
	return s + "/"
}

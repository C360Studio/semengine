package types

// frameworkEntityDomains is the framework-reserved domain set (ADR-102
// decision 4): every framework identity family lives under one of these and
// they pass authorization for every producer. Products delegate their own
// domains; they never re-delegate a reserved one.
var frameworkEntityDomains = [...]string{"agent", "ops", "graph"}

// reservedInstanceTokens are the hierarchy-container padding tokens
// (graph/inference) reserved in the instance position while hierarchy
// containers exist. A producer instance equal to one of them is a corpus finding.
var reservedInstanceTokens = [...]string{"group", "container", "level"}

// IsFrameworkEntityDomain reports whether domain is framework-reserved.
func IsFrameworkEntityDomain(domain string) bool {
	for _, reserved := range frameworkEntityDomains {
		if domain == reserved {
			return true
		}
	}
	return false
}

// IsReservedInstanceToken reports whether token is a reserved instance token.
func IsReservedInstanceToken(token string) bool {
	for _, reserved := range reservedInstanceTokens {
		if token == reserved {
			return true
		}
	}
	return false
}

// EntityDomainDelegation declares that one named producer mints entities under
// an exact entity domain (Type empty) or one exact domain.type. Producer
// identity is supplied by the trusted composition boundary; it is never
// inferred from Triple.Source or a payload type. Wildcards are not supported.
//
// It is a DECLARATION, not an authorization policy. Its consumer is the
// entity-ID corpus audit (not in SemEngine yet), which AST-scans these
// literals in production Go for the registered set its domain_unregistered
// rule consults — no runtime code reads it. There is no authorization policy
// beside it: two producers sharing one domain is PERMITTED, so there is
// nothing to authorize, and detecting a mis-chosen token is a vocabulary
// question rather than a composition-time one.
type EntityDomainDelegation struct {
	Producer string
	Domain   string
	Type     string
}

// validateEntityIDSegment applies the canonical single-segment grammar: one
// non-empty ASCII token beginning alphanumeric with the remaining bytes
// alphanumeric, '_' or '-'. No dots, no wildcards, no rewriting.
func validateEntityIDSegment(value string) error {
	if value == "" {
		return newEntityIDContractError(ErrorCodeEntityIDInvalid, EntityIDReasonEmptySegment, map[string]any{EntityIDDetailSegmentIndex: 0})
	}
	if !isEntityIDAlphanumeric(value[0]) {
		return newEntityIDContractError(ErrorCodeEntityIDInvalid, EntityIDReasonFirstByte, map[string]any{EntityIDDetailSegmentIndex: 0})
	}
	for position := 1; position < len(value); position++ {
		if !isEntityIDRemainingByte(value[position]) {
			return newEntityIDContractError(ErrorCodeEntityIDInvalid, EntityIDReasonAlphabet, map[string]any{EntityIDDetailSegmentIndex: 0})
		}
	}
	return nil
}

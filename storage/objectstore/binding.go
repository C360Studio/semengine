package objectstore

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/c360studio/semengine/component"
)

// consumerHashDomain separates the hashed consumer names from any other SHA-256 use.
const consumerHashDomain = "objectstore-js-consumer-v1"

// inputBinding is how Start binds one input port: core NATS on subject, or a JetStream durable
// consumer named consumer on stream, filtered to subject.
type inputBinding struct {
	port     component.Port
	kind     component.PortKind
	subject  string
	stream   string
	consumer string
}

// planInputBindings plans the binding of every input, refusing two inputs bound to the same
// subject (core NATS) or the same stream and subject (JetStream), naming the ports in sorted
// order. A JetStream input's consumer keeps the legacy name objectstore-<instance>-<subject>, so
// a deployment binds its existing durable consumer again; when two subjects map to one legacy
// name on one stream, each of them gets a name hashed from the instance, stream and subject,
// which no port name or declaration order changes. Carried from the pin's
// planObjectStoreInputBindings; resolvePorts has already checked each port's kind and its one
// subject.
func planInputBindings(inputs []component.Port) ([]inputBinding, error) {
	plans := make([]inputBinding, len(inputs))
	plansByPort := make(map[string]inputBinding, len(inputs))
	duplicatePorts := make(map[string][]string)
	collisionGroups := make(map[string][]int)

	for i, input := range inputs {
		facts, err := input.Facts()
		if err != nil {
			return nil, fmt.Errorf("project input port %q facts: %w", input.Name, err)
		}
		plan := inputBinding{port: input, kind: facts.Kind(), subject: facts.NATSSubjects()[0]}
		switch plan.kind {
		case component.PortKindNATS:
			key := "nats\x00" + plan.subject
			duplicatePorts[key] = append(duplicatePorts[key], input.Name)
		case component.PortKindJetStream:
			stream, ok := facts.Stream()
			if !ok {
				return nil, fmt.Errorf("input port %q has no JetStream declaration", input.Name)
			}
			plan.stream = stream.Name()
			key := "jetstream\x00" + plan.stream + "\x00" + plan.subject
			duplicatePorts[key] = append(duplicatePorts[key], input.Name)
			plan.consumer = legacyConsumerName(plan.subject)
			collision := plan.stream + "\x00" + plan.consumer
			collisionGroups[collision] = append(collisionGroups[collision], i)
		default:
			return nil, fmt.Errorf("input port %q has unsupported kind %q", input.Name, plan.kind)
		}
		plans[i] = plan
		plansByPort[input.Name] = plan
	}

	duplicateKeys := make([]string, 0)
	for key, ports := range duplicatePorts {
		if len(ports) > 1 {
			duplicateKeys = append(duplicateKeys, key)
		}
	}
	sort.Strings(duplicateKeys)
	if len(duplicateKeys) > 0 {
		ports := duplicatePorts[duplicateKeys[0]]
		sort.Strings(ports)
		plan := plansByPort[ports[0]]
		if plan.kind == component.PortKindJetStream {
			return nil, fmt.Errorf("duplicate ObjectStore JetStream binding stream=%q subject=%q declared by ports %q",
				plan.stream, plan.subject, ports)
		}
		return nil, fmt.Errorf("duplicate ObjectStore NATS binding subject=%q declared by ports %q", plan.subject, ports)
	}

	for _, indexes := range collisionGroups {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			plan := &plans[index]
			digest := sha256.Sum256([]byte(consumerHashDomain + "\x00" + instanceName + "\x00" +
				plan.stream + "\x00" + plan.subject))
			plan.consumer = fmt.Sprintf("objectstore-h-%x", digest[:])
		}
	}

	consumerOwners := make(map[string]string)
	for _, plan := range plans {
		if plan.kind != component.PortKindJetStream {
			continue
		}
		if err := validateConsumerName(plan.consumer); err != nil {
			return nil, fmt.Errorf("input port %q planned invalid consumer name %q: %w", plan.port.Name, plan.consumer, err)
		}
		key := plan.stream + "\x00" + plan.consumer
		if owner, duplicate := consumerOwners[key]; duplicate {
			ports := []string{owner, plan.port.Name}
			sort.Strings(ports)
			return nil, fmt.Errorf("ObjectStore JetStream consumer identity collision stream=%q consumer=%q ports=%q",
				plan.stream, plan.consumer, ports)
		}
		consumerOwners[key] = plan.port.Name
	}
	return plans, nil
}

// legacyConsumerName is the consumer name the pin gave a JetStream input on subject.
func legacyConsumerName(subject string) string {
	sanitized := strings.ReplaceAll(subject, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, "*", "all")
	sanitized = strings.ReplaceAll(sanitized, ">", "wildcard")
	return fmt.Sprintf("objectstore-%s-%s", instanceName, sanitized)
}

// validateConsumerName refuses a name NATS would refuse as a durable consumer name.
func validateConsumerName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 255 {
		return fmt.Errorf("name is %d bytes, maximum is 255", len(name))
	}
	if strings.ContainsAny(name, ">*. /\\\t\r\n") {
		return errors.New("name contains a character forbidden by NATS")
	}
	return nil
}

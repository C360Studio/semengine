package metric

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/prometheus/client_golang/prometheus"
	"pgregory.net/rapid"
)

// The generated check for design D9: histories of RegisterOrGet, Unregister and writes through
// returned handles, compared after every step with a reference model the test owns. The model is
// written from D9's rules and Prometheus' registration contract, never from registry.go:
//
//   - a key holds one collector, identified by its concrete type and its descriptor (name, help,
//     label names); a registration under a held key succeeds, returning the held collector, only
//     when both match;
//   - a registration under a free key succeeds only when no collector on the Prometheus registry
//     has that metric name, the core metrics included (Prometheus refuses a second descriptor of
//     one name, whether identical or inconsistent), and when the name's help and label names
//     match every earlier registration of that name: Prometheus keeps them for the registry's
//     lifetime, past Unregister ("dimHashesByName is left untouched", client_golang
//     registry.go, Unregister);
//   - a refusal stores nothing and returns the zero collector with a fatal error; a nil candidate
//     is always refused;
//   - Unregister of a held key frees the key and its name and reports true; of a free key, false;
//   - every write through a handle a successful registration returned is gathered on the key's
//     series until the key is unregistered.
//
// What the eight example tests in registerorget_test.go still own: the six collector kinds one
// by one, a typed-nil *GaugeVec, a collector registered directly on PrometheusRegistry, and the
// concurrent callers (this model is sequential, docs/testing.md).

// propKind is a candidate's concrete type, plus the static type it is registered as.
type propKind int

const (
	kindCounter        propKind = iota // prometheus.NewCounter, registered as Counter
	kindGauge                          // prometheus.NewGauge, registered as Gauge
	kindGaugeAsCounter                 // prometheus.NewGauge, registered as Counter: a gauge satisfies Counter
	kindCounterVec                     // prometheus.NewCounterVec, label "l"
	kindCoreAlias                      // a *GaugeVec with semengine_service_status' descriptor
)

// concrete names the concrete collector type, which is what D9 compares.
func (k propKind) concrete() string {
	switch k {
	case kindCounter:
		return "counter"
	case kindGauge, kindGaugeAsCounter:
		return "gauge"
	case kindCounterVec:
		return "countervec"
	default:
		return "gaugevec"
	}
}

type propSpec struct {
	kind propKind
	name string // fully qualified metric name
	help string
}

// descriptor is the model's descriptor identity: name, help and label names.
func (s propSpec) descriptor() string {
	labels := ""
	if s.kind == kindCounterVec || s.kind == kindCoreAlias {
		labels = "l"
		if s.kind == kindCoreAlias {
			labels = "service"
		}
	}
	return s.name + "|" + s.help + "|" + labels
}

const coreStatusName = "semengine_service_status"
const coreStatusHelp = "Service status (0=stopped, 1=starting, 2=running, 3=stopping, 4=failed)"

// propHandle is a returned collector: its identity, for the canonical check, and how to write once.
type propHandle struct {
	identity any
	write    func()
}

type propHeld struct {
	spec    propSpec
	handles []propHandle // every handle a successful registration of this key returned
	writes  float64
}

type registryMachine struct {
	r     *MetricsRegistry
	keys  map[string]*propHeld // the model: key -> held collector
	names map[string]bool      // the model: metric names on the Prometheus registry
	dims  map[string]string    // the model: name -> help and label names it was first registered with
}

// Activation counters: how often each assertion ran across the whole check (logged, not required).
var propRuns struct {
	sameKeyReturned, sameKeyRefused, newKey, nameRefused, nilRefused, unregTrue, unregFalse, writes, gatheredCompared atomic.Int64
}

func newRegistryMachine() *registryMachine {
	return &registryMachine{
		r:     NewMetricsRegistry(),
		keys:  map[string]*propHeld{},
		names: map[string]bool{coreStatusName: true},
		dims:  map[string]string{coreStatusName: propSpec{kind: kindCoreAlias, name: coreStatusName, help: coreStatusHelp}.descriptor()},
	}
}

var (
	propKeys  = []string{"k0", "k1", "k2"}
	propNames = []string{"d9p_a", "d9p_b"}
	propHelps = []string{"h1", "h2"}
)

func drawSpec(t *rapid.T) propSpec {
	kind := propKind(rapid.IntRange(int(kindCounter), int(kindCoreAlias)).Draw(t, "kind"))
	if kind == kindCoreAlias {
		return propSpec{kind: kind, name: coreStatusName, help: coreStatusHelp}
	}
	return propSpec{
		kind: kind,
		name: rapid.SampledFrom(propNames).Draw(t, "name"),
		help: rapid.SampledFrom(propHelps).Draw(t, "help"),
	}
}

// register calls RegisterOrGet with a fresh candidate of spec and returns the result as a handle.
func (m *registryMachine) register(key string, spec propSpec) (propHandle, error) {
	switch spec.kind {
	case kindCounter:
		c, err := RegisterOrGet(m.r, "svc", key,
			prometheus.NewCounter(prometheus.CounterOpts{Name: spec.name, Help: spec.help}))
		return counterHandle(c), err
	case kindGauge:
		g, err := RegisterOrGet(m.r, "svc", key,
			prometheus.NewGauge(prometheus.GaugeOpts{Name: spec.name, Help: spec.help}))
		if g == nil {
			return propHandle{}, err
		}
		return propHandle{identity: g, write: func() { g.Add(1) }}, err
	case kindGaugeAsCounter:
		var candidate prometheus.Counter = prometheus.NewGauge(prometheus.GaugeOpts{Name: spec.name, Help: spec.help})
		c, err := RegisterOrGet(m.r, "svc", key, candidate)
		return counterHandle(c), err
	case kindCounterVec:
		v, err := RegisterOrGet(m.r, "svc", key,
			prometheus.NewCounterVec(prometheus.CounterOpts{Name: spec.name, Help: spec.help}, []string{"l"}))
		if v == nil {
			return propHandle{}, err
		}
		return propHandle{identity: v, write: func() { v.WithLabelValues("x").Inc() }}, err
	default:
		v, err := RegisterOrGet(m.r, "svc", key,
			prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: spec.name, Help: spec.help}, []string{"service"}))
		if v == nil {
			return propHandle{}, err
		}
		return propHandle{identity: v, write: func() { v.WithLabelValues("prop").Add(1) }}, err
	}
}

func counterHandle(c prometheus.Counter) propHandle {
	if c == nil {
		return propHandle{}
	}
	return propHandle{identity: c, write: func() { c.Inc() }}
}

func (m *registryMachine) RegisterOrGet(t *rapid.T) {
	m.registerAndCompare(t, rapid.SampledFrom(propKeys).Draw(t, "key"), drawSpec(t))
}

// RegisterSame repeats a held key's own spec with a fresh candidate, so the same-key success
// path, which independent draws reach rarely, runs in most histories that hold a key.
func (m *registryMachine) RegisterSame(t *rapid.T) {
	if len(m.keys) == 0 {
		m.RegisterOrGet(t)
		return
	}
	key := rapid.SampledFrom(sortedKeys(m.keys)).Draw(t, "key")
	m.registerAndCompare(t, key, m.keys[key].spec)
}

func (m *registryMachine) registerAndCompare(t *rapid.T, key string, spec propSpec) {
	got, err := m.register(key, spec)

	held, isHeld := m.keys[key]
	switch {
	case isHeld && held.spec.kind.concrete() == spec.kind.concrete() && held.spec.descriptor() == spec.descriptor():
		propRuns.sameKeyReturned.Add(1)
		if err != nil {
			t.Fatalf("same key %s, same type and descriptor: refused: %v", key, err)
		}
		if got.identity != held.handles[0].identity {
			t.Fatalf("same key %s: returned a collector other than the canonical one", key)
		}
		held.handles = append(held.handles, got)
	case isHeld:
		propRuns.sameKeyRefused.Add(1)
		requireRefused(t, err, got, fmt.Sprintf("key %s holds %+v, candidate %+v", key, held.spec, spec))
	case m.names[spec.name]:
		propRuns.nameRefused.Add(1)
		requireRefused(t, err, got, fmt.Sprintf("free key %s, name %s already registered", key, spec.name))
	case m.dims[spec.name] != "" && m.dims[spec.name] != spec.descriptor():
		propRuns.nameRefused.Add(1)
		requireRefused(t, err, got, fmt.Sprintf("free key %s, name %s first registered as %s, candidate %s",
			key, spec.name, m.dims[spec.name], spec.descriptor()))
	default:
		propRuns.newKey.Add(1)
		if err != nil {
			t.Fatalf("free key %s, free name %s: refused: %v", key, spec.name, err)
		}
		m.keys[key] = &propHeld{spec: spec, handles: []propHandle{got}}
		m.names[spec.name] = true
		m.dims[spec.name] = spec.descriptor()
	}
}

func (m *registryMachine) RegisterNil(t *rapid.T) {
	key := rapid.SampledFrom(propKeys).Draw(t, "key")
	var err error
	var got any
	if rapid.Bool().Draw(t, "typedNil") {
		var candidate *prometheus.CounterVec
		got, err = RegisterOrGet(m.r, "svc", key, candidate)
		if got.(*prometheus.CounterVec) != nil {
			t.Fatalf("typed nil returned a collector")
		}
	} else {
		var candidate prometheus.Counter
		var c prometheus.Counter
		c, err = RegisterOrGet(m.r, "svc", key, candidate)
		if c != nil {
			t.Fatalf("nil interface returned a collector")
		}
	}
	propRuns.nilRefused.Add(1)
	if err == nil || !errs.IsFatal(err) {
		t.Fatalf("nil candidate under %s: want a fatal error, got %v", key, err)
	}
}

func (m *registryMachine) Unregister(t *rapid.T) {
	key := rapid.SampledFrom(propKeys).Draw(t, "key")
	held, isHeld := m.keys[key]
	removed := m.r.Unregister("svc", key)
	if removed != isHeld {
		t.Fatalf("Unregister(%s) = %v; the model holds the key: %v", key, removed, isHeld)
	}
	if isHeld {
		propRuns.unregTrue.Add(1)
		delete(m.keys, key)
		delete(m.names, held.spec.name)
	} else {
		propRuns.unregFalse.Add(1)
	}
}

func (m *registryMachine) Write(t *rapid.T) {
	if len(m.keys) == 0 {
		return // nothing returned yet; the step is a no-op, not a skip
	}
	key := rapid.SampledFrom(sortedKeys(m.keys)).Draw(t, "key")
	held := m.keys[key]
	handle := rapid.SampledFrom(held.handles).Draw(t, "handle")
	handle.write()
	held.writes++
	propRuns.writes.Add(1)
}

// Check runs after every step: each held key's series carries exactly the model's writes, and no
// generated name the model does not hold is gathered.
func (m *registryMachine) Check(t *rapid.T) {
	families, err := m.r.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	gathered := map[string]float64{}
	present := map[string]bool{}
	for _, family := range families {
		present[family.GetName()] = true
		for _, metric := range family.Metric {
			gathered[family.GetName()] += seriesValue(family.GetType(), metric)
		}
	}
	for key, held := range m.keys {
		want := held.writes
		propRuns.gatheredCompared.Add(1)
		if gathered[held.spec.name] != want {
			t.Fatalf("key %s (%s): gathered %v, the model's writes %v", key, held.spec.name, gathered[held.spec.name], want)
		}
	}
	for _, name := range propNames {
		if !m.names[name] && present[name] {
			t.Fatalf("%s is gathered but no key holds it", name)
		}
	}
}

func sortedKeys(held map[string]*propHeld) []string {
	keys := make([]string, 0, len(held))
	for _, key := range propKeys {
		if _, ok := held[key]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func requireRefused(t *rapid.T, err error, got propHandle, context string) {
	if err == nil {
		t.Fatalf("%s: accepted; want a refusal", context)
	}
	if !errs.IsFatal(err) {
		t.Fatalf("%s: refusal is not fatal: %v", context, err)
	}
	if got.identity != nil {
		t.Fatalf("%s: a refusal returned a collector", context)
	}
}

// TestPropRegisterOrGetHistory: design D9's generated check (docs/testing.md, "Check a history
// against a reference model").
func TestPropRegisterOrGetHistory(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := newRegistryMachine()
		rt.Repeat(map[string]func(*rapid.T){
			"RegisterOrGet": m.RegisterOrGet,
			"RegisterSame":  m.RegisterSame,
			"RegisterNil":   m.RegisterNil,
			"Unregister":    m.Unregister,
			"Write":         m.Write,
			"":              m.Check,
		})
	})
	t.Logf("assertions run: same-key returned %d, same-key refused %d, new key %d, name refused %d, nil refused %d, "+
		"unregister true %d / false %d, writes %d, gathered compared %d",
		propRuns.sameKeyReturned.Load(), propRuns.sameKeyRefused.Load(), propRuns.newKey.Load(),
		propRuns.nameRefused.Load(), propRuns.nilRefused.Load(), propRuns.unregTrue.Load(),
		propRuns.unregFalse.Load(), propRuns.writes.Load(), propRuns.gatheredCompared.Load())
}

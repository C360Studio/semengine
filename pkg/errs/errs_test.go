package errs

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestErrorClass_String(t *testing.T) {
	tests := []struct {
		class    ErrorClass
		expected string
	}{
		{ErrorTransient, "transient"},
		{ErrorInvalid, "invalid"},
		{ErrorFatal, "fatal"},
		{ErrorClass(999), "unknown"},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := test.class.String()
			if result != test.expected {
				t.Errorf("expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection timeout", ErrConnectionTimeout, true},
		{"connection lost", ErrConnectionLost, true},
		{"storage unavailable", ErrStorageUnavailable, true},
		{"rate limited", ErrRateLimited, true},
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"context canceled", context.Canceled, true},
		{"invalid data", ErrInvalidData, false},
		{"fatal error", ErrResourceExhausted, false},
		{"timeout in message only", fmt.Errorf("operation timeout occurred"), false},
		{"network in message only", fmt.Errorf("network connection failed"), false},
		{"bare error", errors.New("boom"), false},
		{"classified transient", &ClassifiedError{Class: ErrorTransient, Err: fmt.Errorf("test")}, true},
		{"classified fatal", &ClassifiedError{Class: ErrorFatal, Err: fmt.Errorf("test")}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IsTransient(test.err)
			if result != test.expected {
				t.Errorf("expected %v, got %v for error: %v", test.expected, result, test.err)
			}
		})
	}
}

func TestIsFatal(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"invalid config", ErrInvalidConfig, true},
		{"missing config", ErrMissingConfig, true},
		{"data corrupted", ErrDataCorrupted, true},
		{"storage full", ErrStorageFull, true},
		{"resource exhausted", ErrResourceExhausted, true},
		{"quota exceeded", ErrQuotaExceeded, true},
		{"connection timeout", ErrConnectionTimeout, false},
		{"invalid data", ErrInvalidData, false},
		{"fatal in message only", fmt.Errorf("fatal system error occurred"), false},
		{"panic in message only", fmt.Errorf("panic: system failure"), false},
		{"bare error", errors.New("boom"), false},
		{"classified fatal", &ClassifiedError{Class: ErrorFatal, Err: fmt.Errorf("test")}, true},
		{"classified transient", &ClassifiedError{Class: ErrorTransient, Err: fmt.Errorf("test")}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IsFatal(test.err)
			if result != test.expected {
				t.Errorf("expected %v, got %v for error: %v", test.expected, result, test.err)
			}
		})
	}
}

func TestIsInvalid(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"invalid data", ErrInvalidData, true},
		{"parsing failed", ErrParsingFailed, true},
		{"checksum failed", ErrChecksumFailed, true},
		{"connection timeout", ErrConnectionTimeout, false},
		{"fatal error", ErrResourceExhausted, false},
		{"classified invalid", &ClassifiedError{Class: ErrorInvalid, Err: fmt.Errorf("test")}, true},
		{"classified transient", &ClassifiedError{Class: ErrorTransient, Err: fmt.Errorf("test")}, false},
		{"bare error", errors.New("boom"), false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IsInvalid(test.err)
			if result != test.expected {
				t.Errorf("expected %v, got %v for error: %v", test.expected, result, test.err)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorClass
	}{
		{"nil error", nil, ErrorTransient},
		{"connection timeout", ErrConnectionTimeout, ErrorTransient},
		{"invalid config", ErrInvalidConfig, ErrorFatal},
		{"invalid data", ErrInvalidData, ErrorInvalid},
		// An unclassified error is fatal (owner ruling 1, #167 comment 6191970843).
		{"bare error", errors.New("boom"), ErrorFatal},
		{"classified error", &ClassifiedError{Class: ErrorFatal, Err: fmt.Errorf("test")}, ErrorFatal},
		// Text never decides the class (ruling N, #146): an unclassified error with no
		// listed sentinel takes the default, whatever its words, and a wrapped sentinel decides.
		{"timeout and transient words in message only", errors.New("transient timeout reading c360.ops.robotics.gcs.drone.001"), ErrorFatal},
		{"invalid sentinel beside transient words", fmt.Errorf("network timeout: %w", ErrInvalidData), ErrorInvalid},
		// A class or sentinel in the chain keeps marking the error.
		{"wrapped classified transient", fmt.Errorf("handler: %w", &ClassifiedError{Class: ErrorTransient, Err: errors.New("publish")}), ErrorTransient},
		{"wrapped context deadline", fmt.Errorf("handler: %w", context.DeadlineExceeded), ErrorTransient},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Classify(test.err)
			if result != test.expected {
				t.Errorf("expected %v, got %v for error: %v", test.expected, result, test.err)
			}
		})
	}
}

// TestClassifiedErrorWithoutClass pins that a ClassifiedError literal that leaves
// Class unset is not retryable (#258): the zero class names no class, so Classify
// gives it the unclassified default, fatal, even over a transient sentinel.
func TestClassifiedErrorWithoutClass(t *testing.T) {
	var unset ErrorClass
	if got := unset.String(); got != "unknown" {
		t.Errorf("zero ErrorClass String() = %q, want %q", got, "unknown")
	}

	tests := []struct {
		name string
		err  error
	}{
		{"bare cause", &ClassifiedError{Err: errors.New("boom")}},
		{"transient sentinel cause", &ClassifiedError{Err: context.DeadlineExceeded}},
		{"wrapped", fmt.Errorf("handler: %w", &ClassifiedError{Err: errors.New("publish")})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if IsTransient(test.err) || IsInvalid(test.err) || IsFatal(test.err) {
				t.Errorf("an unset class reads as a class: transient=%v invalid=%v fatal=%v",
					IsTransient(test.err), IsInvalid(test.err), IsFatal(test.err))
			}
			if got := Classify(test.err).String(); got != "fatal" {
				t.Errorf("Classify(%v) = %q, want %q", test.err, got, "fatal")
			}
		})
	}
}

func TestClassifiedError(t *testing.T) {
	baseErr := fmt.Errorf("base error")
	ce := newClassified(ErrorTransient, baseErr, "testComponent", "testOperation", "custom message")

	if ce.Class != ErrorTransient {
		t.Errorf("expected ErrorTransient, got %v", ce.Class)
	}

	if ce.Component != "testComponent" {
		t.Errorf("expected testComponent, got %s", ce.Component)
	}

	if ce.Operation != "testOperation" {
		t.Errorf("expected testOperation, got %s", ce.Operation)
	}

	if ce.Error() != "custom message" {
		t.Errorf("expected 'custom message', got %s", ce.Error())
	}

	if !errors.Is(ce, baseErr) {
		t.Error("classified error should unwrap to base error")
	}
}

func TestClassifiedError_NoMessage(t *testing.T) {
	baseErr := fmt.Errorf("base error")
	ce := newClassified(ErrorTransient, baseErr, "testComponent", "testOperation", "")

	if ce.Error() != "base error" {
		t.Errorf("expected 'base error', got %s", ce.Error())
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		component string
		method    string
		action    string
		expected  string
	}{
		{
			"nil error",
			nil,
			"component",
			"method",
			"action",
			"",
		},
		{
			"basic wrap",
			fmt.Errorf("original error"),
			"RoboticsProcessor",
			"processMessage",
			"decode MAVLink",
			"RoboticsProcessor.processMessage: decode MAVLink failed: original error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Wrap(test.err, test.component, test.method, test.action)
			if test.expected == "" {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil || result.Error() != test.expected {
					t.Errorf("expected '%s', got '%v'", test.expected, result)
				}
			}
		})
	}
}

func TestWrapClassified(t *testing.T) {
	baseErr := fmt.Errorf("original error")

	tests := []struct {
		name     string
		wrapFunc func(error, string, string, string) error
		class    ErrorClass
	}{
		{"WrapTransient", WrapTransient, ErrorTransient},
		{"WrapFatal", WrapFatal, ErrorFatal},
		{"WrapInvalid", WrapInvalid, ErrorInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.wrapFunc(baseErr, "component", "method", "action")

			var ce *ClassifiedError
			if !errors.As(result, &ce) {
				t.Error("result should be a ClassifiedError")
				return
			}

			if ce.Class != test.class {
				t.Errorf("expected %v, got %v", test.class, ce.Class)
			}

			if ce.Component != "component" {
				t.Errorf("expected 'component', got %s", ce.Component)
			}

			if ce.Operation != "method" {
				t.Errorf("expected 'method', got %s", ce.Operation)
			}

			if !strings.Contains(ce.Error(), "component.method: action failed") {
				t.Errorf("error should contain standard format, got: %s", ce.Error())
			}
		})
	}
}

func TestWrapClassifiedPreservesMachineContract(t *testing.T) {
	t.Parallel()

	const code = "test_contract"
	detail := map[string]any{"reason": "invalid"}
	inner := ClassifiedCodeDetail(ErrorInvalid, code, detail, errors.New("contract rejected"))

	for _, tc := range []struct {
		name string
		wrap func(error, string, string, string) error
	}{
		{name: "transient", wrap: WrapTransient},
		{name: "fatal", wrap: WrapFatal},
		{name: "invalid", wrap: WrapInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := tc.wrap(inner, "component", "operation", "add context")
			var classified *ClassifiedError
			if !errors.As(wrapped, &classified) {
				t.Fatalf("wrapped error type = %T, want *ClassifiedError", wrapped)
			}
			if classified.Code != code {
				t.Errorf("Code = %q, want %q", classified.Code, code)
			}
			if !reflect.DeepEqual(classified.Detail, detail) {
				t.Errorf("Detail = %#v, want %#v", classified.Detail, detail)
			}
			if !strings.Contains(wrapped.Error(), "component.operation: add context failed") {
				t.Errorf("context missing from %q", wrapped.Error())
			}
		})
	}

	replacement := ClassifiedCode(ErrorInvalid, "replacement_contract", inner)
	if replacement.Code != "replacement_contract" || replacement.Detail != nil {
		t.Errorf("explicit replacement leaked inner contract: Code=%q Detail=%#v", replacement.Code, replacement.Detail)
	}
}

// TestWrapClassified_NilInnerAlwaysReturnsNil verifies that WrapTransient,
// WrapFatal, and WrapInvalid return nil whenever err is nil regardless of
// whether action is empty or non-empty (#218), matching Wrap and Classified.
func TestWrapClassified_NilInnerAlwaysReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		wrapFunc func(error, string, string, string) error
	}{
		{"WrapTransient", WrapTransient},
		{"WrapFatal", WrapFatal},
		{"WrapInvalid", WrapInvalid},
	}

	for _, test := range tests {
		for _, action := range []string{"", "missing required field"} {
			t.Run(fmt.Sprintf("%s_action_%q", test.name, action), func(t *testing.T) {
				if err := test.wrapFunc(nil, "Component", "method", action); err != nil {
					t.Errorf("%s(nil, ..., %q) = %v, want nil", test.name, action, err)
				}
			})
		}
	}
}

// TestWrapClassified_NonNilInnerUnchanged confirms the existing contract
// for the typical case (real upstream error).
func TestWrapClassified_NonNilInnerUnchanged(t *testing.T) {
	upstream := errors.New("upstream failure")
	tests := []struct {
		name     string
		wrapFunc func(error, string, string, string) error
		isClass  func(error) bool
	}{
		{"WrapTransient", WrapTransient, IsTransient},
		{"WrapFatal", WrapFatal, IsFatal},
		{"WrapInvalid", WrapInvalid, IsInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.wrapFunc(upstream, "Component", "method", "action")
			if err == nil {
				t.Fatalf("%s with non-nil inner returned nil", test.name)
			}
			if !errors.Is(err, upstream) {
				t.Errorf("%s: wrapped error should unwrap to upstream, got: %v", test.name, err)
			}
			if !test.isClass(err) {
				t.Errorf("%s: classification lost, got: %v", test.name, err)
			}
		})
	}
}

func TestStandardErrors(t *testing.T) {
	// Test that standard errors are defined
	standardErrors := []error{
		ErrAlreadyStarted,
		ErrNotStarted,
		ErrAlreadyStopped,
		ErrShuttingDown,
		ErrNoConnection,
		ErrConnectionLost,
		ErrConnectionTimeout,
		ErrSubscriptionFailed,
		ErrInvalidData,
		ErrDataCorrupted,
		ErrChecksumFailed,
		ErrParsingFailed,
		ErrStorageFull,
		ErrStorageUnavailable,
		ErrBucketNotFound,
		ErrKeyNotFound,
		ErrInvalidConfig,
		ErrMissingConfig,
		ErrConfigNotFound,
		ErrResourceExhausted,
		ErrRateLimited,
		ErrQuotaExceeded,
		ErrMaxRetriesExceeded,
		ErrRetryTimeout,
	}

	for i, err := range standardErrors {
		if err == nil {
			t.Errorf("standard error at index %d is nil", i)
		}
		if err.Error() == "" {
			t.Errorf("standard error at index %d has empty message", i)
		}
	}
}

// Benchmark error classification performance
func BenchmarkIsTransient(b *testing.B) {
	err := ErrConnectionTimeout
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsTransient(err)
	}
}

func BenchmarkClassify(b *testing.B) {
	err := ErrConnectionTimeout
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Classify(err)
	}
}

func BenchmarkWrap(b *testing.B) {
	err := fmt.Errorf("base error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Wrap(err, "component", "method", "action")
	}
}

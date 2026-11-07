package validation

import "testing"

// TestNotNil covers ordinary nils, typed nils held in interfaces, and valid
// non-nil values.
func TestNotNil(t *testing.T) {
	var typedNil *int
	tests := []struct {
		name    string
		argName string
		value   any
		wantErr bool
	}{
		{name: "non-nil value", argName: "dependency", value: 1},
		{name: "nil interface", argName: "dependency", wantErr: true},
		{name: "typed nil pointer", argName: "dependency", value: typedNil, wantErr: true},
		{name: "blank argument name", argName: "  ", value: 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if gotErr := NotNil(test.argName, test.value) != nil; gotErr != test.wantErr {
				t.Fatalf("NotNil() error present = %t; want %t", gotErr, test.wantErr)
			}
		})
	}
}

// TestNotZero covers zero and non-zero values for both scalar and aggregate
// argument types.
func TestNotZero(t *testing.T) {
	tests := []struct {
		name    string
		argName string
		value   any
		wantErr bool
	}{
		{name: "non-zero scalar", argName: "value", value: 1},
		{name: "non-zero struct", argName: "value", value: struct{ N int }{N: 1}},
		{name: "zero scalar", argName: "value", value: 0, wantErr: true},
		{name: "zero struct", argName: "value", value: struct{ N int }{}, wantErr: true},
		{name: "nil value", argName: "value", wantErr: true},
		{name: "blank argument name", argName: " ", value: 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if gotErr := NotZero(test.argName, test.value) != nil; gotErr != test.wantErr {
				t.Fatalf("NotZero() error present = %t; want %t", gotErr, test.wantErr)
			}
		})
	}
}

// TestNotBlank verifies trimming is returned to callers and blank values or
// names are rejected.
func TestNotBlank(t *testing.T) {
	if got, err := NotBlank(" model ", "  gpt-test  "); err != nil || got != "gpt-test" {
		t.Fatalf("NotBlank() = (%q, %v); want (%q, nil)", got, err, "gpt-test")
	}
	for _, test := range []struct {
		name, argName, value string
	}{
		{name: "blank value", argName: "model", value: " \t"},
		{name: "blank argument name", argName: " ", value: "model"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NotBlank(test.argName, test.value); err == nil {
				t.Fatal("NotBlank() error = nil; want validation error")
			}
		})
	}
}

// TestMinBytes protects the byte-count contract, including multi-byte UTF-8
// input and invalid validator configuration.
func TestMinBytes(t *testing.T) {
	tests := []struct {
		name    string
		argName string
		value   string
		minimum int
		wantErr bool
	}{
		{name: "exact ASCII boundary", argName: "secret", value: "abcd", minimum: 4},
		{name: "multi-byte value", argName: "secret", value: "é", minimum: 2},
		{name: "below byte boundary", argName: "secret", value: "abc", minimum: 4, wantErr: true},
		{name: "non-positive minimum", argName: "secret", value: "abc", wantErr: true},
		{name: "blank argument name", argName: " ", value: "abc", minimum: 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if gotErr := MinBytes(test.argName, test.value, test.minimum) != nil; gotErr != test.wantErr {
				t.Fatalf("MinBytes() error present = %t; want %t", gotErr, test.wantErr)
			}
		})
	}
}

// TestIntegerValidators covers the documented sign boundaries for every
// integer validator.
func TestIntegerValidators(t *testing.T) {
	tests := []struct {
		name    string
		call    func() error
		wantErr bool
	}{
		{name: "positive int", call: func() error { return PositiveInt("count", 1) }},
		{name: "zero int", call: func() error { return PositiveInt("count", 0) }, wantErr: true},
		{name: "positive int64", call: func() error { return PositiveInt64("id", 1) }},
		{name: "negative int64", call: func() error { return PositiveInt64("id", -1) }, wantErr: true},
		{name: "negative non-zero int64", call: func() error { return NonZeroInt64("chat", -1) }},
		{name: "zero non-zero int64", call: func() error { return NonZeroInt64("chat", 0) }, wantErr: true},
		{name: "zero non-negative int64", call: func() error { return NonNegativeInt64("tokens", 0) }},
		{name: "negative non-negative int64", call: func() error { return NonNegativeInt64("tokens", -1) }, wantErr: true},
		{name: "zero non-negative int32", call: func() error { return NonNegativeInt32("requests", 0) }},
		{name: "negative non-negative int32", call: func() error { return NonNegativeInt32("requests", -1) }, wantErr: true},
		{name: "blank name", call: func() error { return PositiveInt(" ", 1) }, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if gotErr := test.call() != nil; gotErr != test.wantErr {
				t.Fatalf("validator error present = %t; want %t", gotErr, test.wantErr)
			}
		})
	}
}

// TestMustValidators verifies each panic wrapper enforces its corresponding
// validation failure.
func TestMustValidators(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{name: "nil", call: func() { MustNotNil("dependency", nil) }},
		{name: "zero", call: func() { MustNotZero("client", 0) }},
		{name: "blank", call: func() { MustNotBlank("prompt", " ") }},
		{name: "minimum bytes", call: func() { MustMinBytes("secret", "short", 32) }},
		{name: "positive int", call: func() { MustPositiveInt("count", 0) }},
		{name: "positive int64", call: func() { MustPositiveInt64("id", 0) }},
		{name: "non-zero int64", call: func() { MustNonZeroInt64("chat", 0) }},
		{name: "non-negative int64", call: func() { MustNonNegativeInt64("tokens", -1) }},
		{name: "non-negative int32", call: func() { MustNonNegativeInt32("requests", -1) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !panics(test.call) {
				t.Fatal("must-validator did not panic")
			}
		})
	}
}

func panics(fn func()) (didPanic bool) {
	defer func() { didPanic = recover() != nil }()
	fn()
	return false
}

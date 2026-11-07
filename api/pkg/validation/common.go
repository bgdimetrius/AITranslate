// Package validation contains transport-independent input validation and
// normalization rules shared by the API's HTTP adapters and services.
package validation

import (
	"fmt"
	"reflect"
	"strings"
)

// NotNil rejects nil values, including interfaces containing typed nil
// pointers. name identifies the argument in the returned error.
func NotNil(name string, value any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if isNil(value) {
		return fmt.Errorf("%s must not be nil", name)
	}
	return nil
}

// MustNotNil panics when value is nil, including a typed nil held in an
// interface.
func MustNotNil(name string, value any) {
	if err := NotNil(name, value); err != nil {
		panic(err)
	}
}

// NotZero rejects a type's zero value. It is useful for value-type clients
// that cannot be checked for nil.
func NotZero(name string, value any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if value == nil || reflect.ValueOf(value).IsZero() {
		return fmt.Errorf("%s must not be its zero value", name)
	}
	return nil
}

// MustNotZero panics when value is its type's zero value.
func MustNotZero(name string, value any) {
	if err := NotZero(name, value); err != nil {
		panic(err)
	}
}

// NotBlank trims and returns value, rejecting an empty result.
func NotBlank(name, value string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("argument name must not be empty")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	return value, nil
}

// MustNotBlank panics when value is empty after trimming and otherwise returns
// its normalized value.
func MustNotBlank(name, value string) string {
	normalized, err := NotBlank(name, value)
	if err != nil {
		panic(err)
	}
	return normalized
}

// MinBytes rejects strings shorter than minimum bytes.
func MinBytes(name, value string, minimum int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if minimum <= 0 {
		return fmt.Errorf("minimum must be positive")
	}
	if len(value) < minimum {
		return fmt.Errorf("%s must contain at least %d bytes", name, minimum)
	}
	return nil
}

// MustMinBytes panics when value is shorter than minimum bytes.
func MustMinBytes(name, value string, minimum int) {
	if err := MinBytes(name, value, minimum); err != nil {
		panic(err)
	}
}

// PositiveInt rejects zero and negative integers.
func PositiveInt(name string, value int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

// MustPositiveInt panics when value is not positive.
func MustPositiveInt(name string, value int) {
	if err := PositiveInt(name, value); err != nil {
		panic(err)
	}
}

// PositiveInt64 rejects zero and negative 64-bit integers.
func PositiveInt64(name string, value int64) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

// MustPositiveInt64 panics when value is not positive.
func MustPositiveInt64(name string, value int64) {
	if err := PositiveInt64(name, value); err != nil {
		panic(err)
	}
}

// NonZeroInt64 rejects zero while allowing signed identifiers such as Telegram
// group chat IDs.
func NonZeroInt64(name string, value int64) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if value == 0 {
		return fmt.Errorf("%s must not be zero", name)
	}
	return nil
}

// MustNonZeroInt64 panics when value is zero.
func MustNonZeroInt64(name string, value int64) {
	if err := NonZeroInt64(name, value); err != nil {
		panic(err)
	}
}

// NonNegativeInt64 rejects negative 64-bit integers.
func NonNegativeInt64(name string, value int64) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if value < 0 {
		return fmt.Errorf("%s must not be negative", name)
	}
	return nil
}

// MustNonNegativeInt64 panics when value is negative.
func MustNonNegativeInt64(name string, value int64) {
	if err := NonNegativeInt64(name, value); err != nil {
		panic(err)
	}
}

// NonNegativeInt32 rejects negative 32-bit integers.
func NonNegativeInt32(name string, value int32) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("argument name must not be empty")
	}
	if value < 0 {
		return fmt.Errorf("%s must not be negative", name)
	}
	return nil
}

// MustNonNegativeInt32 panics when value is negative.
func MustNonNegativeInt32(name string, value int32) {
	if err := NonNegativeInt32(name, value); err != nil {
		panic(err)
	}
}

func isNil(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

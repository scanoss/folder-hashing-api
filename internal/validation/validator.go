// Package validation provides request validation utilities.
package validation

import (
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	once     sync.Once
	instance *validator.Validate
)

// GetValidator returns a singleton validator instance.
func GetValidator() *validator.Validate {
	once.Do(func() {
		instance = validator.New()
	})
	return instance
}

// ValidateStruct validates a struct using the validator instance.
func ValidateStruct(s any) error {
	return GetValidator().Struct(s)
}

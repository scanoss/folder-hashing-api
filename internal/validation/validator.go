package validation

import (
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	once     sync.Once
	instance *validator.Validate
)

func GetValidator() *validator.Validate {
	once.Do(func() {
		instance = validator.New()
	})
	return instance
}

func ValidateStruct(s any) error {
	return GetValidator().Struct(s)
}

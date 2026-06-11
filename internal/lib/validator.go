package lib

import "github.com/go-playground/validator/v10"

type CustomValidator struct {
	v *validator.Validate
}

func NewValidator() *CustomValidator {
	return &CustomValidator{v: validator.New()}
}

func (cv *CustomValidator) Validate(i any) error {
	return cv.v.Struct(i)
}

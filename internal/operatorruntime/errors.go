package operatorruntime

import "errors"

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Message     string       `json:"message"`
	FieldErrors []FieldError `json:"fieldErrors"`
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewValidationError(field, message string) error {
	return &ValidationError{
		Message:     message,
		FieldErrors: []FieldError{{Field: field, Message: message}},
	}
}

func ValidationErrorDetails(err error) (*ValidationError, bool) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) && validationErr != nil {
		return validationErr, true
	}
	return nil, false
}

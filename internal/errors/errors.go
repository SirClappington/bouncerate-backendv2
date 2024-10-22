package errors

import "fmt"

type ErrorType string

const (
	ErrorTypeValidation   ErrorType = "VALIDATION_ERROR"
	ErrorTypeNotFound     ErrorType = "NOT_FOUND"
	ErrorTypeExternal     ErrorType = "EXTERNAL_API_ERROR"
	ErrorTypeInternal     ErrorType = "INTERNAL_ERROR"
	ErrorTypeUnauthorized ErrorType = "UNAUTHORIZED"
)

type APIError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Details any       `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

// Error constructors
func NewValidationError(message string) *APIError {
	return &APIError{
		Type:    ErrorTypeValidation,
		Message: message,
	}
}

func NewExternalError(service string, err error) *APIError {
	return &APIError{
		Type:    ErrorTypeExternal,
		Message: fmt.Sprintf("Error from external service (%s)", service),
		Details: err.Error(),
	}
}

func NewInternalError(err error) *APIError {
	return &APIError{
		Type:    ErrorTypeInternal,
		Message: "Internal server error",
		Details: err.Error(),
	}
}

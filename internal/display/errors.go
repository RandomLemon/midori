package display

import (
	"errors"
	"fmt"
)

var (
	ErrNoBackend            = errors.New("no supported compositor detected")
	ErrConnection           = errors.New("compositor connection failed")
	ErrOutputMissing        = errors.New("output is no longer connected")
	ErrUnsupported          = errors.New("backend does not support this operation")
	ErrInvalidConfiguration = errors.New("invalid display configuration")
)

// ValidationError identifies the output and field that failed validation.
type ValidationError struct {
	OutputID OutputID
	Field    string
	Message  string
}

func (e *ValidationError) Error() string {
	if e.OutputID == "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.OutputID, e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidConfiguration
}

func invalid(id OutputID, field, format string, args ...any) error {
	return &ValidationError{OutputID: id, Field: field, Message: fmt.Sprintf(format, args...)}
}

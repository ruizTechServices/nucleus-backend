package tools

type ErrorKind string

const (
	ErrorKindValidation ErrorKind = "validation"
	ErrorKindDenied     ErrorKind = "denied"
	ErrorKindNotFound   ErrorKind = "not_found"
)

type CallError struct {
	Kind    ErrorKind
	Message string
	Details map[string]any
}

func (e *CallError) Error() string {
	if e == nil {
		return ""
	}

	return e.Message
}

func NewValidationError(message string, details map[string]any) *CallError {
	return &CallError{
		Kind:    ErrorKindValidation,
		Message: message,
		Details: details,
	}
}

func NewDeniedError(message string, details map[string]any) *CallError {
	return &CallError{
		Kind:    ErrorKindDenied,
		Message: message,
		Details: details,
	}
}

func NewNotFoundError(message string, details map[string]any) *CallError {
	return &CallError{
		Kind:    ErrorKindNotFound,
		Message: message,
		Details: details,
	}
}

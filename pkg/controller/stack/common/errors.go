package common

import "strings"

//CompoundError is the sum of multiple errors e.g. for validation purposes.
type CompoundError struct {
	message string
	elements []error
}

// Error() implements the error interface.
func (e *CompoundError) Error() string {
	return e.message
}

//NewCompoundError creates a compound error from the given slice of errors.
func NewCompoundError(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	msg := strings.Builder{}
	for i, err := range errs {
		msg.WriteString(err.Error())
		if i + 1 < len(errs) {
			msg.WriteString("; ")
		}
	}
	return &CompoundError{
		message: msg.String(),
		elements: errs,
	}
}

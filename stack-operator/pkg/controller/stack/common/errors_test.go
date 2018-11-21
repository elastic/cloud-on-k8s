package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCompoundError(t *testing.T) {
	sampleErrors := []error{errors.New("a"), errors.New("b")}
	tests := []struct {
		name string
		args []error
		want error
	}{
		{
			name: "multiple errors are folded into one",
			args: sampleErrors,
			want: &CompoundError{
				message:  "a; b",
				elements: sampleErrors,
			},
		},
		{
			name: "no errors don't create an error ",
			args: []error{},
			want: nil,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, NewCompoundError(tt.args))
	}
}

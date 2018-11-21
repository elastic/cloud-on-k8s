package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringInSlice(t *testing.T) {
	type args struct {
		str  string
		list []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"String in slice returns true",
			args{
				"info",
				[]string{"info", "warn", "debug"},
			},
			true,
		},
		{
			"String not in slice returns false",
			args{
				"error",
				[]string{"info", "warn", "debug"},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringInSlice(tt.args.str, tt.args.list)
			assert.Equal(t, tt.want, got)
		})
	}
}

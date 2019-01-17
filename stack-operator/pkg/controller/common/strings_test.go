package common

import (
	"testing"

	"github.com/stretchr/testify/require"

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

func TestRemoveStringInSlice(t *testing.T) {
	type args struct {
		s     string
		slice []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "removes string from slice",
			args: args{
				s:     "b",
				slice: []string{"a", "b", "c"},
			},
			want: []string{"a", "c"},
		},
		{
			name: "removes string from slice multiple occurrences",
			args: args{
				s:     "b",
				slice: []string{"a", "b", "c", "b"},
			},
			want: []string{"a", "c"},
		},
		{
			name: "noop when string not found",
			args: args{
				s:     "d",
				slice: []string{"a", "b", "c"},
			},
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ElementsMatch(t, tt.want, RemoveStringInSlice(tt.args.s, tt.args.slice))
		})
	}
}

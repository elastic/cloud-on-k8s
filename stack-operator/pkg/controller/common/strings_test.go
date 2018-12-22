package common

import (
	"reflect"
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

func TestRemoveStringInSlice(t *testing.T) {
	type args struct {
		r    string
		list []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "removes string from slice",
			args: args{
				r:    "b",
				list: []string{"a", "b", "c"},
			},
			want: []string{"a", "c"},
		},
		{
			name: "removes string from slice multiple occurrences",
			args: args{
				r:    "b",
				list: []string{"a", "b", "c", "b"},
			},
			want: []string{"a", "c"},
		},
		{
			name: "noop when string not found",
			args: args{
				r:    "d",
				list: []string{"a", "b", "c"},
			},
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveStringInSlice(tt.args.r, tt.args.list); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoveStringInSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testLabel TrueFalseLabel = "foo"

func TestTrueFalseLabel_Set(t *testing.T) {
	type args struct {
		value  bool
		labels map[string]string
	}
	tests := []struct {
		name string
		l    TrueFalseLabel
		args args
		want map[string]string
	}{
		{
			name: "true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{},
			},
			want: map[string]string{"foo": "true"},
		},
		{
			name: "talse",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{},
			},
			want: map[string]string{"foo": "false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.l.Set(tt.args.value, tt.args.labels)
			assert.Equal(t, tt.want, tt.args.labels)
		})
	}
}

func TestTrueFalseLabel_HasValue(t *testing.T) {
	type args struct {
		value  bool
		labels map[string]string
	}
	tests := []struct {
		name string
		l    TrueFalseLabel
		args args
		want bool
	}{
		{
			name: "unset, true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{},
			},
			want: false,
		},
		{
			name: "unset, false",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{},
			},
			want: false,
		},
		{
			name: "set unexpected, true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{"foo": "unexpected"},
			},
			want: false,
		},
		{
			name: "set unexpected, false",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{"foo": "unexpected"},
			},
			want: false,
		},
		{
			name: "set true, true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{"foo": "true"},
			},
			want: true,
		},
		{
			name: "set true, false",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{"foo": "false"},
			},
			want: false,
		},
		{
			name: "set false, false",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{"foo": "false"},
			},
			want: true,
		},
		{
			name: "set false, true",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{"foo": "true"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.HasValue(tt.args.value, tt.args.labels); got != tt.want {
				t.Errorf("TrueFalseLabel.HasValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrueFalseLabel_AsMap(t *testing.T) {
	type args struct {
		value bool
	}
	tests := []struct {
		name string
		l    TrueFalseLabel
		args args
		want map[string]string
	}{
		{
			name: "true",
			l:    testLabel,
			args: args{value: true},
			want: map[string]string{"foo": "true"},
		},
		{
			name: "false",
			l:    testLabel,
			args: args{value: false},
			want: map[string]string{"foo": "false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.l.AsMap(tt.args.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

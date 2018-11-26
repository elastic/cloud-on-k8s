package flex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFailure(t *testing.T) {
	type args struct {
		msg string
	}
	tests := []struct {
		name string
		args args
		want Response
	}{
		{
			name: "failure",
			args: args{msg: "some failure occured"},
			want: Response{
				Status:  StatusFailure,
				Message: "some failure occured",
			},
		},
		{
			name: "failure 2",
			args: args{msg: "some other failure occured"},
			want: Response{
				Status:  StatusFailure,
				Message: "some other failure occured",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Failure(tt.args.msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSuccess(t *testing.T) {
	type args struct {
		msg string
	}
	tests := []struct {
		name string
		args args
		want Response
	}{
		{
			name: "success",
			args: args{msg: "some success occured"},
			want: Response{
				Status:  StatusSuccess,
				Message: "some success occured",
			},
		},
		{
			name: "success 2",
			args: args{msg: "some other success occured"},
			want: Response{
				Status:  StatusSuccess,
				Message: "some other success occured",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Success(tt.args.msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNotSupported(t *testing.T) {
	type args struct {
		msg string
	}
	tests := []struct {
		name string
		args args
		want Response
	}{
		{
			name: "notSupported",
			args: args{msg: "some notSupported occured"},
			want: Response{
				Status:  StatusNotSupported,
				Message: "some notSupported occured",
			},
		},
		{
			name: "notSupported 2",
			args: args{msg: "some other notSupported occured"},
			want: Response{
				Status:  StatusNotSupported,
				Message: "some other notSupported occured",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NotSupported(tt.args.msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

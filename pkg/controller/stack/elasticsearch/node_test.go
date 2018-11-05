package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNodeName(t *testing.T) {
	type args struct {
		clusterName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Generates a random name from a short stack name",
			args: args{
				clusterName: "some-stack-name",
			},
			want: "some-stack-name-es-(.*)",
		},
		{
			name: "Generates a random name from a long stack name",
			args: args{
				clusterName: "some-stack-name-that-is-quite-long-and-will-be-trimmed",
			},
			want: "some-stack-name-that-is-quite-long-and-will-be-tr-es-(.*)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewNodeName(tt.args.clusterName)
			if len(tt.args.clusterName) > maxPrefixLength {
				assert.Len(t, got, 63, got, "should be maximum 63 characters long")
			}

			assert.Regexp(t, tt.want, got)
		})
	}
}

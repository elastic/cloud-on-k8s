package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicServiceURL(t *testing.T) {
	type args struct {
		stackName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL",
			args: args{stackName: "a-stack-name"},
			want: "http://a-stack-name-es-public:9200",
		},
		{
			name: "Another Service URL",
			args: args{stackName: "another-stack-name"},
			want: "http://another-stack-name-es-public:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PublicServiceURL(tt.args.stackName)
			assert.Equal(t, tt.want, got)
		})
	}
}

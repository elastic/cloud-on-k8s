package kibana

import "testing"

func TestNewDeploymentName(t *testing.T) {
	type args struct {
		stackName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{stackName: "a-stack-name"},
			want: "a-stack-name-kibana",
		},
		{
			args: args{stackName: "another-stack-name"},
			want: "another-stack-name-kibana",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewDeploymentName(tt.args.stackName); got != tt.want {
				t.Errorf("NewDeploymentName() = %v, want %v", got, tt.want)
			}
		})
	}
}

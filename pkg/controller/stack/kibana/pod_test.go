package kibana

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPodSpecDefaults(t *testing.T) {
	actual := NewPodSpec(PodSpecParams{})
	expected := imageWithVersion(defaultImageRepositoryAndName, "")
	for _, c := range actual.Containers {
		if c.Image != expected {
			t.Errorf("NewPodSpec with defaults: expected %s, actual %s ", expected, c.Image)
		}
	}
}

func TestNewPodSpecOverrides(t *testing.T) {
	params := PodSpecParams{CustomImageName: "my-custom-image:1.0.0", Version: "7.0.0"}
	actual := NewPodSpec(params)
	expected := params.CustomImageName
	for _, c := range actual.Containers {
		if c.Image != expected {
			t.Errorf("NewPodSpec with custom image: expected %s, actual %s", expected, c.Image)
		}
	}
}

func Test_imageWithVersion(t *testing.T) {
	type args struct {
		image   string
		version string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{image: "someimage", version: "6.4.2"},
			want: "someimage:6.4.2",
		},
		{
			args: args{image: "differentimage", version: "6.4.1"},
			want: "differentimage:6.4.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageWithVersion(tt.args.image, tt.args.version)
			assert.Equal(t, tt.want, got)
		})
	}
}

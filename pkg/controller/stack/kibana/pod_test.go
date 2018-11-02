package kibana

import "testing"

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

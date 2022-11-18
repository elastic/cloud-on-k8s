package preflight

import (
	"context"
	"errors"
	"fmt"

	plibRuntime "github.com/redhat-openshift-ecosystem/openshift-preflight/certification/runtime"
	container "github.com/redhat-openshift-ecosystem/openshift-preflight/container"
)

var ErrImageEmpty = errors.New("image is empty")

type RunInput struct {
	Image                  string
	DockerConfigPath       string
	PyxisAPIToken          string
	CertificationProjectID string
}

// Run will run the preflight checks for a given image name.
func Run(ctx context.Context, input RunInput) (plibRuntime.Results, error) {
	if input.Image == "" {
		return plibRuntime.Results{}, ErrImageEmpty
	}

	check := container.NewCheck(
		input.Image,
		container.WithDockerConfigJSONFromFile(input.DockerConfigPath),
		container.WithCertificationProject(input.CertificationProjectID, input.PyxisAPIToken),
	)

	res, err := check.Run(ctx)
	if err != nil {
		return plibRuntime.Results{}, fmt.Errorf("while running preflight checks for %s: %w", input.Image, err)
	}
	return res, nil
}

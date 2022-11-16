package preflight

import (
	"context"
	"errors"
	"fmt"

	plib "github.com/redhat-openshift-ecosystem/openshift-preflight"
	"github.com/redhat-openshift-ecosystem/openshift-preflight/certification/policy"
	plibRuntime "github.com/redhat-openshift-ecosystem/openshift-preflight/certification/runtime"
)

var ErrImageEmpty = errors.New("image is empty")

// Run will run the preflight checks for a given image name.
func Run(ctx context.Context, image, dockerConfig, pyxisAPIToken string) (plibRuntime.Results, error) {
	if image == "" {
		return plibRuntime.Results{}, ErrImageEmpty
	}

	cfg := plibRuntime.Config{
		Image:          image,
		ResponseFormat: "json",
		Policy:         policy.PolicyContainer,
		PyxisAPIToken:  pyxisAPIToken,
		WriteJUnit:     false,
		Submit:         false,
		DockerConfig:   dockerConfig,
	}

	check := plib.NewContainerCheck(image, plib.WithRuntimeConfig(cfg))

	res, err := check.Run(ctx)
	if err != nil {
		return plibRuntime.Results{}, fmt.Errorf("while running preflight checks for %s: %w", image, err)
	}
	return res, nil
}

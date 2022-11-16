package preflight

import (
	"context"
	"fmt"

	plib "github.com/sebrandon1/openshift-preflight"
	plibRuntime "github.com/sebrandon1/openshift-preflight/certification/runtime"
)

// Run will run the preflight checks for a given image name.
func Run(image string) (plibRuntime.Results, error) {
	check := plib.NewContainerCheck(image)
	results, err := check.Run(context.TODO())
	if err != nil {
		return results, fmt.Errorf("while running preflight checks for %s: %w", image, err)
	}
	return results, nil
}

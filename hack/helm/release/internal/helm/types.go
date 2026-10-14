// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v4/pkg/registry"
)

// chart defines the elements of a Helm chart.
type chart struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Dependencies []dependency `json:"dependencies"`
	srcPath      string
}

// packagedChart is a Helm chart packaged into a chart archive at packagePath.
type packagedChart struct {
	chart
	packagePath string
}

// dependency is a dependency of a Helm chart.
type dependency struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository"`
}

// ociPusher is the subset of registry.Client used to look up and push charts in the OCI registry, allowing injection of test doubles.
type ociPusher interface {
	Resolve(ref string) (ocispec.Descriptor, error)
	Push(data []byte, ref string, opts ...registry.PushOption) (*registry.PushResult, error)
}

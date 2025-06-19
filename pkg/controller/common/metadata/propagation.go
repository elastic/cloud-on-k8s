// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package metadata

import (
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	eckmaps "github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// Metadata is a container for labels and annotations.
type Metadata struct {
	Labels      map[string]string
	Annotations map[string]string
}

// Merge the given set of metadata with the existing ones.
func (md Metadata) Merge(other Metadata) Metadata {
	return Metadata{
		Annotations: eckmaps.Merge(maps.Clone(md.Annotations), other.Annotations),
		Labels:      eckmaps.Merge(maps.Clone(md.Labels), other.Labels),
	}
}

// Propagate returns a new set of metadata to apply to child objects.
// Behaviour is driven by the presence of annotation and label propagation annotations in the object.
// Elements chosen for propagation are merged with the elements to add giving precedence to the latter.
func Propagate(obj metav1.Object, toAdd Metadata) Metadata {
	inherited := annotation.GetMetadataToPropagate(obj)
	return Metadata{
		Annotations: merge(toAdd.Annotations, inherited.Annotations),
		Labels:      merge(toAdd.Labels, inherited.Labels),
	}
}

func merge(toAdd map[string]string, inherited map[string]string) map[string]string {
	newElements := maps.Clone(toAdd)
	if len(inherited) == 0 {
		return newElements
	}
	return eckmaps.MergePreservingExistingKeys(newElements, inherited)
}

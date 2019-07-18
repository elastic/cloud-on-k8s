// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// ControllerVersionAnnotation is the annotation name that indicates the last controller version to update a resource
const ControllerVersionAnnotation = "common.k8s.elastic.co/controller-version"

// UpdateControllerVersion updates the controller version annotation to the current version if necessary
func UpdateControllerVersion(client k8s.Client, obj runtime.Object, version string) error {
	accessor := meta.NewAccessor()
	namespace, err := accessor.Namespace(obj)
	if err != nil {
		log.Error(err, "error getting namespace", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return err
	}
	name, err := accessor.Name(obj)
	if err != nil {
		log.Error(err, "error getting name", "namespace", namespace, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return err
	}
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		log.Error(err, "error getting annotations", "namespace", namespace, "name", name, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return err
	}
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// do not send unnecessary update if the value would not change
	if annotations[ControllerVersionAnnotation] == version {
		log.V(1).Info("Skipping controller version annotation update, version already matches", "namespace", namespace, "name", name, "kind", obj.GetObjectKind())
		return nil
	}

	annotations[ControllerVersionAnnotation] = version
	err = accessor.SetAnnotations(obj, annotations)
	if err != nil {
		log.Error(err, "error updating controller version annotation", "namespace", namespace, "name", name, "kind", obj.GetObjectKind())
		return err
	}
	log.V(1).Info("updating controller version annotation", "namespace", namespace, "name", name, "kind", obj.GetObjectKind())
	return client.Update(obj)
}

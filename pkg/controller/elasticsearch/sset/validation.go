// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodTemplateError struct {
	Parent      metav1.Object
	StatefulSet appsv1.StatefulSet
	Causes      []metav1.StatusCause
}

func (e *PodTemplateError) Error() string {
	return fmt.Sprintf(
		"Validation of PodTemplate for StatefulSet %s in Elasticsearch %s/%s failed for the following reasons: %v",
		e.StatefulSet.Name,
		e.Parent.GetNamespace(),
		e.Parent.GetName(),
		e.Causes,
	)
}

// validatePodTemplate validates a Pod Template by issuing a dry API request.
func validatePodTemplate(
	c k8s.Client,
	parent metav1.Object,
	sset appsv1.StatefulSet,
) error {
	template := sset.Spec.Template
	// Create a dummy Pod with the pod template
	dummyPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   sset.GetNamespace(),
			Name:        sset.GetName() + "-dummy-" + rand.String(5),
			Labels:      template.Labels,
			Annotations: template.Annotations,
		},
		Spec: template.Spec,
	}
	if err := c.Create(dummyPod, client.DryRunAll); err != nil {
		return toPodTemplateError(parent, sset, err)
	}
	return nil
}

// toPodTemplateError attempts to extract the meaningful information from the dry run API call by converting the original
// error into a PodTemplateError.
func toPodTemplateError(
	parent metav1.Object,
	sset appsv1.StatefulSet,
	err error,
) error {
	var statusErr *k8serrors.StatusError
	if !errors.As(err, &statusErr) {
		// Not a Kubernetes API error (e.g. timeout)
		return err
	}

	switch statusErr.ErrStatus.Reason {
	case metav1.StatusReasonBadRequest:
		// Dry run is beta and available since Kubernetes 1.13
		// Openshift 3.11 and K8S 1.12 don't support dryRun but returns "400 - BadRequest" in that case
		return nil
	case metav1.StatusReasonInvalid:
		// If the Pod spec is invalid the expected error is 422.
		// Since "details" is a pointer let's check that it's not nil before going further.
		if statusErr.ErrStatus.Details == nil {
			return statusErr
		}
		// We are only interested in the causes, other information is not relevant since it is a "dummy" Pod
		return &PodTemplateError{
			Parent:      parent,
			StatefulSet: sset,
			Causes:      statusErr.ErrStatus.Details.Causes,
		}
	}
	// The Kubernetes API returns an error which is not related to the spec. of the Pod, let's retry again later
	return statusErr
}

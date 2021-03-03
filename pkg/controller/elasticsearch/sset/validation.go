// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"context"
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

// validatePodTemplate validates a Pod Template by issuing a dry-run API request.
// This check is performed as "best-effort" for the following reasons:
// * It is only supported by the API server starting 1.13
// * There might be some admission webhooks on the validation path that are not compatible with dry-run requests.
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
	if err := c.Create(context.Background(), dummyPod, client.DryRunAll); err != nil {
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

	if statusErr.ErrStatus.Reason == metav1.StatusReasonInvalid {
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

	// The Kubernetes API returns an error which is not related to the spec. of the Pod. In order to not block
	// the reconciliation loop we skip the validation.
	log.Info("Pod validation skipped", "namespace", parent.GetNamespace(), "es_name", parent.GetName(), "error", statusErr)
	return nil
}

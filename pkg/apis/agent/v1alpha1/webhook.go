// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

const (
	// WebhookPath is the HTTP path for the Elastic Agent validating webhook.
	WebhookPath = "/validate-agent-k8s-elastic-co-v1alpha1-agent"

	MissingPolicyIDMessage = "spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"
)

var groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

// +kubebuilder:webhook:path=/validate-agent-k8s-elastic-co-v1alpha1-agent,mutating=false,failurePolicy=ignore,groups=agent.k8s.elastic.co,resources=agents,verbs=create;update,versions=v1alpha1,name=elastic-agent-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate validates an Agent resource, given an optional old Agent for update checks.
func Validate(a *Agent, old *Agent) (admission.Warnings, error) {
	return a.validate(old)
}

func (a *Agent) warnings() []string {
	if a == nil {
		return nil
	}
	var warnings []string
	if a.Spec.Mode == AgentFleetMode && len(a.Spec.PolicyID) == 0 {
		warnings = append(warnings, fmt.Sprintf("%s %s/%s: %s", Kind, a.Namespace, a.Name, MissingPolicyIDMessage))
	}
	if pt, path := a.activePodTemplate(); path != "" {
		if w := commonv1.PodTemplateResourcesOverrideWarning("spec.resources", path, AgentContainerName, a.Spec.Resources, pt); w != "" {
			warnings = append(warnings, w)
		}
	}
	return warnings
}

// activePodTemplate returns the configured pod template and its spec path.
// checkSpec ensures at most one deployment mode is set.
// If no deployment mode (DaemonSet, Deployment, or StatefulSet) is set, it returns an empty PodTemplateSpec and an empty path.
func (a *Agent) activePodTemplate() (corev1.PodTemplateSpec, string) {
	switch {
	case a.Spec.DaemonSet != nil:
		return a.Spec.DaemonSet.PodTemplate, "spec.daemonSet.podTemplate"
	case a.Spec.Deployment != nil:
		return a.Spec.Deployment.PodTemplate, "spec.deployment.podTemplate"
	case a.Spec.StatefulSet != nil:
		return a.Spec.StatefulSet.PodTemplate, "spec.statefulSet.podTemplate"
	}
	return corev1.PodTemplateSpec{}, ""
}

func (a *Agent) validate(old *Agent) (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

	deprecationWarnings, deprecationErrors := checkIfVersionDeprecated(a)
	if deprecationErrors != nil {
		errors = append(errors, deprecationErrors...)
	}
	if deprecationWarnings != "" {
		warnings = append(warnings, deprecationWarnings)
	}
	warnings = append(warnings, a.warnings()...)

	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, a); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return warnings, apierrors.NewInvalid(groupKind, a.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(a); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, a.Name, errors)
	}
	return warnings, nil
}

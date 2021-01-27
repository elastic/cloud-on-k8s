// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package rbac

import (
	"context"
	"strings"

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/gobuffalo/flect"
	authorizationapi "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
)

const (
	ServiceAccountUsernamePrefix = "system:serviceaccount:"
)

var log = ulog.Log.WithName("access-review")

type AccessReviewer interface {
	// AccessAllowed checks that the given ServiceAccount is allowed to get an other object.
	AccessAllowed(ctx context.Context, serviceAccount string, sourceNamespace string, object runtime.Object) (bool, error)
}

type SubjectAccessReviewer struct {
	client kubernetes.Interface
}

var _ AccessReviewer = &SubjectAccessReviewer{}

func NewSubjectAccessReviewer(client kubernetes.Interface) AccessReviewer {
	return &SubjectAccessReviewer{
		client: client,
	}
}

func NewPermissiveAccessReviewer() AccessReviewer {
	return &permissiveAccessReviewer{}
}

func (s *SubjectAccessReviewer) AccessAllowed(ctx context.Context, serviceAccount string, sourceNamespace string, object runtime.Object) (bool, error) {
	metaObject, err := meta.Accessor(object)
	if err != nil {
		return false, nil
	}
	// For convenience we still allow association between objects in a same namespace
	if sourceNamespace == metaObject.GetNamespace() {
		return true, nil
	}

	if len(serviceAccount) == 0 {
		serviceAccount = "default"
	}

	allErrs := field.ErrorList{}
	// This validation could be done in other places but it is important to be sure that it is done before any access review.
	for _, msg := range validation.IsDNS1123Subdomain(serviceAccount) {
		allErrs = append(allErrs, &field.Error{Type: field.ErrorTypeInvalid, Field: "serviceAccount", BadValue: serviceAccount, Detail: msg})
	}
	if len(allErrs) > 0 {
		return false, allErrs.ToAggregate()
	}

	sar := newSubjectAccessReview(metaObject, object, serviceAccount, sourceNamespace)

	sar, err = s.client.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	log.V(1).Info(
		"Access review", "result", sar.Status,
		"service_account", serviceAccount,
		"source_namespace", sourceNamespace,
		"remote_kind", object.GetObjectKind().GroupVersionKind().Kind,
		"remote_namespace", metaObject.GetNamespace(),
		"remote_name", metaObject.GetName(),
	)
	if sar.Status.Denied {
		return false, nil
	}
	return sar.Status.Allowed, nil
}

func newSubjectAccessReview(
	metaObject metav1.Object,
	object runtime.Object,
	serviceAccount, sourceNamespace string,
) *authorizationapi.SubjectAccessReview {
	return &authorizationapi.SubjectAccessReview{
		Spec: authorizationapi.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationapi.ResourceAttributes{
				Namespace: metaObject.GetNamespace(),
				Verb:      "get",
				Resource:  strings.ToLower(flect.Pluralize(object.GetObjectKind().GroupVersionKind().Kind)),
				Group:     strings.ToLower(object.GetObjectKind().GroupVersionKind().Group),
				Version:   strings.ToLower(object.GetObjectKind().GroupVersionKind().Version),
				Name:      metaObject.GetName(),
			},
			User: ServiceAccountUsernamePrefix + sourceNamespace + ":" + serviceAccount,
		},
	}
}

type permissiveAccessReviewer struct{}

var _ AccessReviewer = &permissiveAccessReviewer{}

func (s *permissiveAccessReviewer) AccessAllowed(_ context.Context, _ string, _ string, _ runtime.Object) (bool, error) {
	return true, nil
}

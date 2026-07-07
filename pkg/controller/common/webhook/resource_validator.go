// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// ValidateFunc is the per-resource validation callback.
// obj is the object being validated, old is nil/zero on create.
type ValidateFunc[T runtime.Object] func(obj T, old T) (admission.Warnings, error)

// ResourceValidator implements admission.Validator[T] by wrapping an inner
// validator with namespace filtering and license checking. It can wrap either
// a simple ValidateFunc (via funcValidator) or a full admission.Validator[T].
type ResourceValidator[T runtime.Object] struct {
	validator         admission.Validator[T]
	managedNamespaces set.StringSet
	licenseChecker    license.Checker
	namespaceMatcher  *nsmatch.NamespaceMatcher
}

// NewResourceValidator wraps an admission.Validator[T] with namespace
// filtering and license checking. This is currently used for Logstash,
// Elasticsearch, ElasticsearchAutoscaling, and AutoOps only.
//
// Pass nil for licenseChecker only when the inner validator performs license
// enforcement itself (so ResourceValidator.preValidate does not run a second
// check).
func NewResourceValidator[T runtime.Object](
	licenseChecker license.Checker,
	managedNamespaces []string,
	validator admission.Validator[T],
) *ResourceValidator[T] {
	return &ResourceValidator[T]{
		validator:         validator,
		managedNamespaces: set.Make(managedNamespaces...),
		licenseChecker:    licenseChecker,
	}
}

// NewResourceFuncValidator wraps a ValidateFunc[T] with namespace filtering
// and license checking. This is the common case for CRDs outside of
// Logstash, Elasticsearch, ElasticsearchAutoscaling, and AutoOps.
func NewResourceFuncValidator[T runtime.Object](
	licenseChecker license.Checker,
	managedNamespaces []string,
	validate ValidateFunc[T],
) *ResourceValidator[T] {
	return NewResourceValidator(licenseChecker, managedNamespaces, &funcValidator[T]{validate: validate})
}

// WithNamespaceMatcher attaches a NamespaceMatcher so that dynamic-mode
// installs can filter validation by the operator's current selector instead
// of the static managedNamespaces list.
func (v *ResourceValidator[T]) WithNamespaceMatcher(m *nsmatch.NamespaceMatcher) *ResourceValidator[T] {
	v.namespaceMatcher = m
	return v
}

func (v *ResourceValidator[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	if skip, err := v.preValidate(ctx, obj); skip || err != nil {
		return nil, err
	}
	return v.validator.ValidateCreate(ctx, obj)
}

func (v *ResourceValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	if skip, err := v.preValidate(ctx, newObj); skip || err != nil {
		return nil, err
	}
	return v.validator.ValidateUpdate(ctx, oldObj, newObj)
}

func (v *ResourceValidator[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	if skip, _ := v.preValidate(ctx, obj); skip {
		return nil, nil
	}
	return v.validator.ValidateDelete(ctx, obj)
}

// preValidate checks namespace filtering and license requirements.
// Returns (true, nil) to skip validation (allow), or (false, error) to deny.
// When the object is in an unmanaged namespace, no errors/warnings are returned.
func (v *ResourceValidator[T]) preValidate(ctx context.Context, obj T) (skip bool, err error) {
	whlog := ulog.FromContext(ctx).WithName("common-webhook")

	accessor := meta.NewAccessor()
	ns, _ := accessor.Namespace(obj)
	name, _ := accessor.Name(obj)

	if v.namespaceMatcher.SelectorEnabled() {
		// The webhook server runs in a single operator pod but receives admission requests
		// for every namespace in the cluster: several operator instances, each watching a
		// different set of namespaces, may be installed side by side, but only one of their
		// webhooks is actually registered. Matches consults the match-state map, which is
		// maintained on every replica — including the non-leader one that may be serving
		// this webhook. Namespaces that do not match, including those not yet observed,
		// are silently let through instead of denied, so an operator never rejects a
		// request for a namespace it doesn't manage.
		if !v.namespaceMatcher.Matches(ns) {
			whlog.V(1).Info("Skip resource validation: namespace does not match selector", "name", name, "namespace", ns)
			return true, nil
		}
	} else if v.managedNamespaces.Count() > 0 && !v.managedNamespaces.Has(ns) {
		whlog.V(1).Info("Skip resource validation", "name", name, "namespace", ns)
		return true, nil
	}

	if v.licenseChecker != nil {
		// If the license check fails, the request is denied and
		// no further warnings/errors from validations are returned.
		errorList := hasRequestedLicenseLevel(ctx, obj, v.licenseChecker)
		if len(errorList) > 0 {
			gvk := obj.GetObjectKind().GroupVersionKind()
			gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
			return false, apierrors.NewInvalid(gk, name, errorList)
		}
	}

	return false, nil
}

// RegisterResourceWebhook creates a ResourceValidator wrapping a ValidateFunc
// and registers it as a validating webhook at the specified path.
func RegisterResourceWebhook[T runtime.Object](mgr ctrl.Manager, path string, checker license.Checker, managedNamespaces []string, matcher *nsmatch.NamespaceMatcher, validate ValidateFunc[T], resourceName string) {
	v := NewResourceFuncValidator(checker, managedNamespaces, validate).WithNamespaceMatcher(matcher)
	wh := admission.WithValidator[T](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(path, wh)
	ulog.Log.Info("Registering validating webhook", "resource", resourceName, "path", path)
}

// funcValidator implements admission.Validator[T] by delegating to a ValidateFunc.
type funcValidator[T runtime.Object] struct {
	validate ValidateFunc[T]
}

// ValidateCreate validates a new object. The "old" parameter passed to the
// underlying ValidateFunc is the zero value of T. For pointer types (*Agent,
// *ApmServer, etc.) this is nil, so nil-checks in validation code work as
// expected. For value types, the zero value would be a real struct, so this
// implementation assumes T is always a pointer type.
func (f *funcValidator[T]) ValidateCreate(_ context.Context, obj T) (admission.Warnings, error) {
	var zero T
	return f.validate(obj, zero)
}

func (f *funcValidator[T]) ValidateUpdate(_ context.Context, oldObj, newObj T) (admission.Warnings, error) {
	return f.validate(newObj, oldObj)
}

func (f *funcValidator[T]) ValidateDelete(_ context.Context, obj T) (admission.Warnings, error) {
	return nil, nil
}

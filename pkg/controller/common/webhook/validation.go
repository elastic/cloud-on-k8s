// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

func hasRequestedLicenseLevel(ctx context.Context, obj runtime.Object, checker license.Checker) field.ErrorList {
	whlog := ulog.FromContext(ctx).WithName("common-webhook")
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		whlog.Error(err, "while accessing runtime object metadata")
		return nil // we do not want to block admission because of it
	}
	var errs field.ErrorList
	ok, err := license.HasRequestedLicenseLevel(ctx, annotations, checker)
	if err != nil {
		whlog.Error(err, "while checking license level during validation")
		return nil
	}
	if !ok {
		errs = append(errs, field.Invalid(field.NewPath("metadata").Child("annotations").Child(license.Annotation), "enterprise", "Enterprise license required but ECK operator is running on a Basic license"))
	}
	return errs
}

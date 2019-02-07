// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	deploymentsv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
)

// StackID returns the qualified identifier for this stack deployment
// based on the given namespace and stack name, following
// the convention: <namespace>-<stack name>
func StackID(s deploymentsv1alpha1.Stack) string {
	return stringsutil.Concat(s.Namespace, "-", s.Name)
}

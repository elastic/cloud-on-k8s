// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func init() {
	Register(operator.GlobalOperator, func(mgr manager.Manager, params Parameters) error {
		return RegisterValidations(mgr, params)
	})
}

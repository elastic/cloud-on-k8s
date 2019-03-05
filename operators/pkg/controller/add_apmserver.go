// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/apmserver"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	Register(operator.NamespaceOperator, apmserver.Add)
}

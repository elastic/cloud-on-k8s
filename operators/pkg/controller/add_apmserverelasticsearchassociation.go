// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserverelasticsearchassociation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
)

func init() {
	Register(operator.NamespaceOperator, apmserverelasticsearchassociation.Add)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

func PseudoNamespacedResourceName(as v1alpha1.ApmServer) string {
	return stringsutil.Concat(as.Name, "-apm-server")
}

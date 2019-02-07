// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
)

func PseudoNamespacedResourceName(kb v1alpha1.Kibana) string {
	return stringsutil.Concat(kb.Name, "-kibana")
}

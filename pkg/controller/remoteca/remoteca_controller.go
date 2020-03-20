// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/remotecluster/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	name = "remoteca-controller"
)

// Add creates a new RemoteCa Controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := remoteca.NewReconciler(mgr, accessReviewer, params)
	c, err := common.NewController(mgr, name, r, params)
	if err != nil {
		return err
	}
	return remoteca.AddWatches(c, r)
}

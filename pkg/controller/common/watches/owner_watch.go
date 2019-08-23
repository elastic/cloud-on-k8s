// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

type OwnerWatch struct {
	handler.EnqueueRequestForOwner
}

func (o *OwnerWatch) Key() string {
	return o.OwnerType.GetObjectKind().GroupVersionKind().Kind + "-owner"
}

func (o *OwnerWatch) EventHandler() handler.EventHandler {
	return o
}

var _ HandlerRegistration = &OwnerWatch{}

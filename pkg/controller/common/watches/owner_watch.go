// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

type OwnerWatch[T client.Object] struct {
	Scheme       *runtime.Scheme
	Mapper       meta.RESTMapper
	OwnerType    client.Object
	IsController bool
}

func (o *OwnerWatch[T]) Key() string {
	return o.OwnerType.GetObjectKind().GroupVersionKind().Kind + "-owner"
}

func (o *OwnerWatch[T]) EventHandler() handler.TypedEventHandler[T] {
	opts := []handler.OwnerOption{}
	if o.IsController {
		opts = []handler.OwnerOption{handler.OnlyControllerOwner()}
	}

	return handler.TypedEnqueueRequestForOwner[T](o.Scheme, o.Mapper, o.OwnerType, opts...)
}

var _ HandlerRegistration[client.Object] = &OwnerWatch[client.Object]{}

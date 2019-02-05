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

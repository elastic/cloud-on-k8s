package finalizer

import (
	"context"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("finalizer")

// Finalizer can be attached to a resource and executed upon resource deletion
type Finalizer struct {
	Name    string
	Execute func() error
}

// Handler handles registration and execution of finalizers
// Note that it is not thread-safe.
type Handler struct {
	client client.Client
}

// NewHandler creates a Handler
func NewHandler(client client.Client) Handler {
	return Handler{
		client: client,
	}
}

// Handle the configured finalizers for the given resource.
// The given objectMeta must be a sub-part of the given resource: updates for the
// resource will be issued against the apiserver based on objectMeta content if needed.
// If the resource is marked for deletion, finalizers will be executed then removed
// from the resource.
// Else, the function is making sure all finalizers are correctly registered for the resource.
func (h *Handler) Handle(objectMeta *metav1.ObjectMeta, resource runtime.Object, finalizers ...Finalizer) error {
	if !objectMeta.DeletionTimestamp.IsZero() {
		// resource is being deleted, let's execute finalizers
		return h.executeFinalizers(finalizers, objectMeta, resource)
	}
	// resource is not being deleted, make sure all finalizers are there
	return h.reconcileFinalizers(finalizers, objectMeta, resource)
}

// ReconcileFinalizers makes sure all finalizers exist in the given objectMeta.
// If some finalizers need to be added to objectMeta,
// an update to the apiserver will be issued for the given resource.
func (h *Handler) reconcileFinalizers(finalizers []Finalizer, objectMeta *metav1.ObjectMeta, resource runtime.Object) error {
	needUpdate := false
	for _, finalizer := range finalizers {
		// add finalizer if not already there
		if !common.StringInSlice(finalizer.Name, objectMeta.Finalizers) {
			log.Info("Registering finalizer", "name", finalizer.Name)
			objectMeta.Finalizers = append(objectMeta.Finalizers, finalizer.Name)
			needUpdate = true
		}
	}
	if needUpdate {
		return h.client.Update(context.TODO(), resource)
	}
	return nil
}

// executeFinalizers runs all registered finalizers in the given objectMeta.
// Once a finalizer is executed, it is removed from the objectMeta's list,
// and an update to the apiserver is issued for the given resource.
func (h *Handler) executeFinalizers(finalizers []Finalizer, objectMeta *metav1.ObjectMeta, resource runtime.Object) error {
	needUpdate := false
	var finalizerErr error
	for _, finalizer := range finalizers {
		// for each registered finalizer, execute it, then remove from the list
		if common.StringInSlice(finalizer.Name, objectMeta.Finalizers) {
			log.Info("Executing finalizer", "name", finalizer.Name)
			if finalizerErr = finalizer.Execute(); finalizerErr != nil {
				break
			}
			needUpdate = true
			objectMeta.Finalizers = common.RemoveString(objectMeta.Finalizers, finalizer.Name)
		}
	}
	if needUpdate {
		if err := h.client.Update(context.TODO(), resource); err != nil {
			return err
		}
	}
	return finalizerErr
}

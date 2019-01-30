package finalizer

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	client k8s.Client
}

// NewHandler creates a Handler
func NewHandler(client k8s.Client) Handler {
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
func (h *Handler) Handle(resource runtime.Object, finalizers ...Finalizer) error {
	metaObject, err := meta.Accessor(resource)
	if err != nil {
		return err
	}
	var needUpdate bool
	var finalizerErr error
	if metaObject.GetDeletionTimestamp().IsZero() {
		// resource is not being deleted, make sure all finalizers are there
		needUpdate = h.reconcileFinalizers(finalizers, metaObject, resource)
	} else {
		// resource is being deleted, let's execute finalizers
		needUpdate, finalizerErr = h.executeFinalizers(finalizers, metaObject, resource)
	}
	if needUpdate {
		if updateErr := h.client.Update(resource); updateErr != nil {
			return updateErr
		}
	}
	return finalizerErr
}

// ReconcileFinalizers makes sure all finalizers exist in the given objectMeta.
// If some finalizers need to be added to objectMeta,
// an update to the apiserver will be issued for the given resource.
func (h *Handler) reconcileFinalizers(finalizers []Finalizer, object metav1.Object, resource runtime.Object) bool {
	needUpdate := false
	for _, finalizer := range finalizers {
		// add finalizer if not already there
		if !common.StringInSlice(finalizer.Name, object.GetFinalizers()) {
			log.Info("Registering finalizer", "name", finalizer.Name)
			object.SetFinalizers(append(object.GetFinalizers(), finalizer.Name))
			needUpdate = true
		}
	}
	return needUpdate
}

// executeFinalizers runs all registered finalizers in the given objectMeta.
// Once a finalizer is executed, it is removed from the objectMeta's list,
// and an update to the apiserver is issued for the given resource.
func (h *Handler) executeFinalizers(finalizers []Finalizer, object metav1.Object, resource runtime.Object) (bool, error) {
	needUpdate := false
	var finalizerErr error
	for _, finalizer := range finalizers {
		// for each registered finalizer, execute it, then remove from the list
		if common.StringInSlice(finalizer.Name, object.GetFinalizers()) {
			log.Info("Executing finalizer", "name", finalizer.Name)
			if finalizerErr = finalizer.Execute(); finalizerErr != nil {
				break
			}
			needUpdate = true
			object.SetFinalizers(common.RemoveStringInSlice(finalizer.Name, object.GetFinalizers()))
		}
	}
	return needUpdate, finalizerErr
}

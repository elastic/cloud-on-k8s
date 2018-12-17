package reconciler

import (
	"context"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("generic-reconciler")
)

type Params struct {
	client.Client
	Scheme *runtime.Scheme
	Owner  metav1.Object
	Object runtime.Object
	// func(expected, found T) bool
	Differ interface{}
	// func(expected, found T)
	Modifier interface{}
}

func ReconcileResource(params Params) error {
	obj := params.Object
	meta, ok := obj.(metav1.Object)
	if !ok {
		return errors.Errorf("%v is not a k8s metadata Object", obj)
	}
	namespace := meta.GetNamespace()
	name := meta.GetName()
	kinds, unversioned, err := params.Scheme.ObjectKinds(obj)
	kind := "unknown"
	if !unversioned && err == nil {
		kind = kinds[0].Kind
	}
	if err := controllerutil.SetControllerReference(params.Owner, meta, params.Scheme); err != nil {
		return err
	}
	// Check if already exists
	expected := obj
	resourceType := reflect.Indirect(reflect.ValueOf(obj)).Type()
	empty := reflect.New(resourceType)
	found, ok := empty.Interface().(runtime.Object)
	if !ok {
		return errors.Errorf("%v was not a k8s runtime.Object", obj)
	}
	log.V(3).Info(fmt.Sprintf("GET args %s/%s:  %v", name, namespace, reflect.ValueOf(found)))
	err = params.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating %s %s/%s", kind, namespace, name))

		err = params.Create(context.TODO(), expected)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Generic GET failed with error")
		return err
	}

	//TODO panics included from here on + add better error messages
	v := reflect.ValueOf(params.Differ)
	funcArgs := []reflect.Value{
		reflect.ValueOf(expected),
		reflect.ValueOf(found),
	}
	updateNeeded := v.Call(funcArgs)[0].Bool()

	// Update if needed
	if updateNeeded {
		log.Info(fmt.Sprintf("Updating %s %s/%s ", kind, namespace, name))
		v = reflect.ValueOf(params.Modifier)
		v.Call(funcArgs)
		err := params.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	}
	reflect.ValueOf(found).MethodByName("DeepCopyInto").Call([]reflect.Value{reflect.ValueOf(expected)})
	return nil
}

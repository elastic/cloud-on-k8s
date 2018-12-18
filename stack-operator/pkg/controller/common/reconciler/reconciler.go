package reconciler

import (
	"context"
	"fmt"
	"reflect"
	"strings"

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

// Params is a parameter object for the ReconcileResources function
type Params struct {
	client.Client
	Scheme *runtime.Scheme
	// Owner will be set as the controller reference
	Owner metav1.Object
	// Object the resource to reconcile
	Object runtime.Object
	// Differ is a generic function of the type func(expected, found T) bool where T is a runtime.Object
	Differ interface{}
	// Modifier is generic function of the type func(expected, found T) where T is runtime Object
	Modifier interface{}
}

// ReconcileResource implements a generic reconciliation function for runtime.Objects.
func ReconcileResource(params Params) error {
	meta, ok := params.Object.(metav1.Object)
	if !ok {
		return errors.Errorf("%v is not a k8s metadata Object", params.Object)
	}
	namespace := meta.GetNamespace()
	name := meta.GetName()
	kinds, _, err := params.Scheme.ObjectKinds(params.Object)
	if err != nil {
		return err
	}
	kind := kinds[0].Kind

	if err := controllerutil.SetControllerReference(params.Owner, meta, params.Scheme); err != nil {
		return err
	}

	// runtime.Object is an interface containing a pointer to value of type resourceType
	resourceType := reflect.Indirect(reflect.ValueOf(params.Object)).Type()
	empty := reflect.New(resourceType)
	found, ok := empty.Interface().(runtime.Object)
	if !ok {
		return errors.Errorf("%v was not a k8s runtime.Object", params.Object)
	}
	// Check if already exists
	err = params.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating %s %s/%s", kind, namespace, name))

		err = params.Create(context.TODO(), params.Object)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Generic GET failed with error")
		return err
	}

	v := reflect.ValueOf(params.Differ)
	var updateNeeded bool
	var funcArgs = []reflect.Value{
		reflect.ValueOf(params.Object),
		reflect.ValueOf(found),
	}

	if v.IsValid() {
		validateDiffer(v, resourceType)
		updateNeeded = v.Call(funcArgs)[0].Bool()
	} else {
		// if Differ is absent we fall back to a simple deep equal
		updateNeeded = !reflect.DeepEqual(params.Object, found)
	}

	// Update if needed
	if updateNeeded {
		log.Info(fmt.Sprintf("Updating %s %s/%s ", kind, namespace, name))
		v = reflect.ValueOf(params.Modifier)
		validateModifier(v, resourceType)
		v.Call(funcArgs)
		err := params.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	}
	reflect.ValueOf(found).MethodByName("DeepCopyInto").Call([]reflect.Value{reflect.ValueOf(params.Object)})
	return nil
}

func validateModifier(mod reflect.Value, resourceType reflect.Type) {
	validateDynamic("Modifier", mod, resourceType, nil)
}

func validateDiffer(differ reflect.Value, resourceType reflect.Type) {
	returns := reflect.TypeOf(true)
	validateDynamic("Differ", differ, resourceType, &returns)
}

func validateDynamic(name string, fn reflect.Value, resourceType reflect.Type, returns *reflect.Type) {
	msg := fmt.Sprintf("%s needs to be of type func(expected, found *%s) %v", name, resourceType, *returns)

	if !fn.IsValid() || fn.Kind() != reflect.Func {
		panic(msg)
	}

	funcType := fn.Type()
	if funcType.NumIn() != 2 {
		panic(msg)
	}

	var incorrectArgs []string
	for i := 0; i < funcType.NumIn(); i++ {
		if !funcType.In(i).AssignableTo(reflect.PtrTo(resourceType)) {
			incorrectArgs = append(incorrectArgs, funcType.In(i).String())
		}
	}

	if len(incorrectArgs) > 0 {
		panic(msg + " got incorrect args: " + strings.Join(incorrectArgs, ", "))
	}

	var returnTypes []reflect.Type
	for i := 0; i < funcType.NumOut(); i++ {
		returnTypes = append(returnTypes, funcType.Out(i))
	}
	hasIncorrectReturns := len(returnTypes) != 0
	if returns != nil {
		hasIncorrectReturns = len(returnTypes) != 1 || (len(returnTypes) > 0 && returnTypes[0] != *returns)
	}

	if hasIncorrectReturns {
		panic(msg + fmt.Sprintf(" incorrect return types %v ", returnTypes))
	}
}

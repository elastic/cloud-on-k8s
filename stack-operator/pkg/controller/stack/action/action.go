package action

import (
	"context"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/state"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	// NOOP action does nothign
	NOOP = noop{}
	log  = logf.Log.WithName("action")
)

// Context contains reconciliation loop iteration specific context
// needed to execute an action
type Context struct {
	client.Client
	State     state.ReconcileState
	Iteration int64
}

// Interface is an action to take after a reconciliation attempt
type Interface interface {
	Name() string
	Execute(ctx Context) (*reconcile.Result, error)
}

// Apply the given actions to the initial state
func Apply(ctx Context, actions []Interface) (state.ReconcileState, error) {
	var applied []Interface
	for _, action := range actions {
		log.Info("Running " + action.Name())
		result, err := action.Execute(ctx)
		if err != nil {
			return ctx.State, err
		}
		applied = append(applied, action)
		if result != nil {
			newState := ctx.State
			newState.Result = *result
			return newState, nil
		}
	}
	return ctx.State, nil
}

type noop struct{}

func (n noop) Name() string {
	return "NOOP"
}

func (n noop) Execute(ctx Context) (*reconcile.Result, error) {
	return nil, nil
}

// Create action
type Create struct {
	Obj runtime.Object
}

// Name of the action
func (c Create) Name() string {
	name := "Create"
	meta, ok := c.Obj.(v1.Object)
	if ok {
		name = common.Concat(name, " ", c.Obj.GetObjectKind().GroupVersionKind().Kind, meta.GetNamespace(), "/", meta.GetName())
	}
	return name
}

// Execute to run the action
func (c Create) Execute(ctx Context) (*reconcile.Result, error) {
	log.Info(c.Name(), "iteration", ctx.Iteration)
	err := ctx.Create(context.TODO(), c.Obj)
	return nil, err
}

// Update action
type Update struct {
	Obj runtime.Object
}

// Name of the action
func (c Update) Name() string {
	name := "Update"
	meta, ok := c.Obj.(v1.Object)
	if ok {
		name = common.Concat(name, " ", c.Obj.GetObjectKind().GroupVersionKind().Kind, meta.GetNamespace(), "/", meta.GetName())
	}
	return name
}

// Execute to run the action
func (c Update) Execute(ctx Context) (*reconcile.Result, error) {
	log.Info(c.Name(), "iteration", ctx.Iteration)
	err := ctx.Update(context.TODO(), c.Obj)
	return nil, err
}

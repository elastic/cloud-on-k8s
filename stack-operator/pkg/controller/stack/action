package action

import (
	"context"

	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/state"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	NOOP = noop{}
	log  = logf.Log.WithName("action")
)

type Context struct {
	client.Client
	State     state.ReconcileState
	Iteration int64
}

// Interface is an action to take after a reconciliation attempt
type Interface interface {
	Name() string
	Execute(ctx Context) (*reconcile.Result, error)
	Compensate() error
}

// Unapply the given actions by calling their compensate function
func Unapply(actions []Interface) {
	for _, action := range actions {

		if action.Compensate() != nil {
			err := action.Compensate()
			if err != nil {
				panic(err) //what else to do here?
			}
		}
	}
}

// Apply the given actions to the initial state
func Apply(ctx Context, actions []Interface) state.ReconcileState {
	var applied []Interface
	for _, action := range actions {
		log.Info("Running " + action.Name())
		result, err := action.Execute(ctx)
		if err != nil {
			Unapply(applied)
			break
		}
		applied = append(applied, action)
		if result != nil {
			newState := ctx.State
			newState.Result = *result
			return newState
		}
	}
	return ctx.State
}

type noop struct{}

func (n noop) Name() string {
	return "NOOP"
}

func (n noop) Execute(ctx Context) (*reconcile.Result, error) {
	return nil, nil
}

func (n noop) Compensate() error {
	return nil
}

// Create action
type Create struct {
	Interface
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
	Interface
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

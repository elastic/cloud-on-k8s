package action

import (
	"context"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/state"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	// NOOP action does nothing
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

// Builder is a helper to create actions
type Builder struct {
	actions []Interface
	errors  []error
}

// Add a action and check for errors.
func (b *Builder) Add(action Interface, err error) *Builder {
	if err != nil {
		b.errors = append(b.errors, err)
	}
	b.actions = append(b.actions, action)
	return b
}

// AddN adds multiple actions and checks for errors
func (b *Builder) AddN(actions []Interface, err error) *Builder {
	if err != nil {
		b.errors = append(b.errors, err)
	}
	b.actions = append(b.actions, actions...)
	return b
}

// Apply all the actions in this builder
func (b *Builder) Apply(ctx Context) (state.ReconcileState, error) {
	if len(b.errors) > 0 {
		return ctx.State, common.NewCompoundError(b.errors)
	}
	return apply(ctx, b.actions)

}

// Info returns a descriptive string about this action builder.
func (b *Builder) Info() string {
	var str strings.Builder

	for i, e := range b.errors {
		str.WriteString(e.Error())
		if i+1 < len(b.errors) {
			str.WriteString(", ")
		}
	}

	str.WriteString("Actions [")
	for i, a := range b.actions {
		if a == NOOP {
			continue
		}
		str.WriteString(a.Name())
		if i+1 < len(b.actions) {
			str.WriteString(", ")
		}
	}
	str.WriteString("]")
	return str.String()

}

func nextTakesPrecendence(current reconcile.Result, next reconcile.Result) bool {
	if current == next {
		return false // no need to replace the result
	}
	if next.Requeue && !current.Requeue && current.RequeueAfter <= 0 {
		return true // next requests requeue current does not, next takes precendence
	}
	if next.RequeueAfter > 0 && (current.RequeueAfter == 0 || next.RequeueAfter < current.RequeueAfter) {
		return true // next requests a requeue and current does not or wants it only later
	}
	return false //default case

}

func apply(ctx Context, actions []Interface) (state.ReconcileState, error) {
	var applied []Interface
	newState := ctx.State
	for _, action := range actions {
		nextResult, err := action.Execute(ctx)
		if err != nil {
			return ctx.State, err
		}
		applied = append(applied, action)
		if nextResult != nil && nextTakesPrecendence(newState.Result, *nextResult) {
			newState.Result = *nextResult
		}
	}
	return newState, nil
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
		name = common.Concat(
			name, " ", c.Obj.GetObjectKind().GroupVersionKind().Kind,
			meta.GetNamespace(), "/", meta.GetName(),
		)
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
		name = common.Concat(
			name, " ", c.Obj.GetObjectKind().GroupVersionKind().Kind,
			meta.GetNamespace(), "/", meta.GetName(),
		)
	}
	return name
}

// Execute to run the action
func (c Update) Execute(ctx Context) (*reconcile.Result, error) {
	log.Info(c.Name(), "iteration", ctx.Iteration)
	err := ctx.Update(context.TODO(), c.Obj)
	return nil, err
}

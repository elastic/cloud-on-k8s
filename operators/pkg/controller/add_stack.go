package controller

import (
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/stack"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, stack.Add)
}

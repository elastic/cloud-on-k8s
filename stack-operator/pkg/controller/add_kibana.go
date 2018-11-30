package controller

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/kibana"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, kibana.Add)
}

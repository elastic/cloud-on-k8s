package controller

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, operator.Parameters) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, params operator.Parameters) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m, params); err != nil {
			return err
		}
	}
	return nil
}

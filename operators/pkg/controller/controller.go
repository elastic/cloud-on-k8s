package controller

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs = make(map[string][]func(manager.Manager, operator.Parameters) error)

// Register a controller for a specific manager role.
func Register(role string, add func(manager.Manager, operator.Parameters) error) {
	fns := AddToManagerFuncs[role]
	AddToManagerFuncs[role] = append(fns, add)

}

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, role string, params operator.Parameters) error {
	for k, fs := range AddToManagerFuncs {
		if role == operator.All || k == role {
			for _, f := range fs {
				if err := f(m, params); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

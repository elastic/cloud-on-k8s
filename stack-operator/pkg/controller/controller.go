package controller

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, net.Dialer) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, dialer net.Dialer) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m, dialer); err != nil {
			return err
		}
	}
	return nil
}

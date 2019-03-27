// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs = make(map[string][]func(manager.Manager, Parameters) error)

// Register a controller for a specific manager role.
func Register(role string, add func(manager.Manager, Parameters) error) {
	fns := AddToManagerFuncs[role]
	AddToManagerFuncs[role] = append(fns, add)

}

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, role string, paramsFn func() (*Parameters, error)) error {
	var params *Parameters
	var err error
	for k, fs := range AddToManagerFuncs {
		if role == operator.All || k == role {
			if params == nil {
				// lazily initialize params so that errors happen only if we actually want to use a webhook
				params, err = paramsFn()
				if err != nil {
					return err
				}
			}
			for _, f := range fs {
				if err := f(m, *params); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

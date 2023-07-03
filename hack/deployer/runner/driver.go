// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"reflect"

	"dario.cat/mergo"
)

const (
	CreateAction = "create"
	DeleteAction = "delete"
)

var (
	drivers = make(map[string]DriverFactory)
)

// DriverFactory allows creating a driver
type DriverFactory interface {
	Create(Plan) (Driver, error)
}

// Driver allows executing a plan
type Driver interface {
	// Execute runs the plan configured during driver creation.
	Execute() error
	// GetCredentials updates a kubeconfig file with appropriate credentials for the current environment.
	GetCredentials() error
}

func GetPlan(plans []Plan, config RunConfig, clientBuildDefDir string) (Plan, error) {
	plan, err := choosePlan(plans, config.Id)
	if err != nil {
		return Plan{}, err
	}

	plan, err = merge(plan, config.Overrides)
	if err != nil {
		return Plan{}, err
	}

	// allows plans and runConfigs to set this value but use a default if not set
	if plan.ClientBuildDefDir == "" {
		plan.ClientBuildDefDir = clientBuildDefDir
	}

	return plan, nil
}

// GetDriver picks plan based on the run config and returns the appropriate driver
func GetDriver(plans []Plan, config RunConfig, clientBuildDefDir string) (Driver, error) {
	plan, err := GetPlan(plans, config, clientBuildDefDir)
	if err != nil {
		return nil, err
	}

	driverFactory, err := chooseFactory(plan.Provider)
	if err != nil {
		return nil, err
	}

	return driverFactory.Create(plan)
}

func choosePlan(plans []Plan, id string) (Plan, error) {
	for _, p := range plans {
		if p.Id == id {
			return p, nil
		}
	}

	return Plan{}, fmt.Errorf("no plan with id %s found", id)
}

func merge(base Plan, overrides map[string]interface{}) (Plan, error) {
	// mergo will not override with empty values which is inconvenient for booleans, hence custom transformer
	err := mergo.Map(&base, overrides, mergo.WithOverride, mergo.WithTransformers(boolTransformer{}))
	return base, err
}

type boolTransformer struct {
}

func (bt boolTransformer) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	var b bool
	if typ == reflect.TypeOf(b) {
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				dst.SetBool(src.Interface().(bool)) //nolint:forcetypeassert
			}
			return nil
		}
	}
	return nil
}

func chooseFactory(provider string) (DriverFactory, error) {
	driverFactory, ok := drivers[provider]
	if !ok {
		return nil, fmt.Errorf("no driver for provider with id %s found", provider)
	}

	return driverFactory, nil
}

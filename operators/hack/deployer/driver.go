// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"reflect"

	"github.com/imdario/mergo"
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
	Execute() error
}

// GetDriver picks plan based on the run config and returns the appropriate driver
func GetDriver(plans []Plan, config RunConfig) (Driver, error) {
	plan, err := choosePlan(plans, config.Id)
	if err != nil {
		return nil, err
	}

	plan, err = merge(plan, config.Overrides)
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
				dst.SetBool(src.Interface().(bool))
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

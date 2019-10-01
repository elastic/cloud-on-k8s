// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/Jeffail/gabs"
	"sigs.k8s.io/yaml"
)

const (
	crdsDirectory = "../../config/crds"
	pathToRemove  = "spec.validation.openAPIV3Schema.properties.spec.properties.podTemplate"
)

func main() {
	crds, err := ioutil.ReadDir(crdsDirectory)
	if err != nil {
		log.Fatal(err)
	}

	for _, crd := range crds {
		crdPath := filepath.Join(crdsDirectory, crd.Name())
		err := patchCRD(crdPath)
		if err != nil {
			log.Fatalf("Fail to patch crd %s: %s", crdPath, err.Error())
		}
	}
}

// patchCRD removes the element pointed by the `pathToRemove` if this path exists.
// It converts the YAML in JSON in order to use github.com/Jeffail/gabs which is
// a pretty convenient library to manipulate an arbitrary JSON.
func patchCRD(filename string) error {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	jsonBytes, err := yaml.YAMLToJSON(bytes)
	if err != nil {
		return err
	}

	jsonObj, err := gabs.ParseJSON(jsonBytes)
	if err != nil {
		return err
	}

	// Skip if the element to remove does not exist (this is the case for the ES CRD)
	if !jsonObj.ExistsP(pathToRemove) {
		return nil
	}

	err = jsonObj.DeleteP(pathToRemove)
	if err != nil {
		return err
	}

	yamlBytes, err := yaml.JSONToYAML(jsonObj.Bytes())
	if err != nil {
		return err
	}

	// Append the --- yaml separator like the controller-gen
	// https://github.com/kubernetes-sigs/controller-tools/blob/master/pkg/genall/genall.go#L106
	err = ioutil.WriteFile(filename, append([]byte("\n---\n"), yamlBytes...), 0644)
	if err != nil {
		return err
	}

	return nil
}

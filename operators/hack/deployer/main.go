// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/ghodss/yaml"
)

func main() {
	plansFile := flag.String("plans-file", "config/plans.yml", "File containing execution plans.")
	configFile := flag.String("run-config-file", "config/run-config.yml", "File containing run config.")
	flag.Parse()

	plans, err := parsePlans(*plansFile)
	if err != nil {
		log.Printf("error %s while trying to parse plans file at %s", err, *plansFile)
		os.Exit(1)
	}

	runConfig, err := parseRunConfig(*configFile)
	if err != nil {
		log.Printf("error %s while trying to parse runConfig file at %s", err, *configFile)
		os.Exit(1)
	}

	err = run(plans, runConfig)
	if err != nil {
		log.Printf("error %s while running plans at %s with runConfig at %s", err, *plansFile, *configFile)
		os.Exit(1)
	}
}

func run(plans Plans, runConfig RunConfig) error {
	driver, err := GetDriver(plans.Plans, runConfig)
	if err != nil {
		return err
	}

	return driver.Execute()
}

func parsePlans(path string) (plans Plans, err error) {
	yml, err := ioutil.ReadFile(path)
	if err == nil {
		err = yaml.Unmarshal(yml, &plans)
	}

	return
}

func parseRunConfig(path string) (runConfig RunConfig, err error) {
	yml, err := ioutil.ReadFile(path)
	if err == nil {
		err = yaml.Unmarshal(yml, &runConfig)
	}

	return
}

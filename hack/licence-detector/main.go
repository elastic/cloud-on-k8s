// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/dependency"
	"github.com/elastic/cloud-on-k8s/hack/licence-detector/detector"
	"github.com/elastic/cloud-on-k8s/hack/licence-detector/render"
	"github.com/elastic/cloud-on-k8s/hack/licence-detector/validate"
)

var (
	depsTemplateFlag    = flag.String("depsTemplate", "templates/dependencies.asciidoc.tmpl", "Path to the dependency list template file")
	depsOutFlag         = flag.String("depsOut", "dependencies.asciidoc", "Path to output the dependency list")
	inFlag              = flag.String("in", "-", "Dependency list (output from go list -m -json all)")
	includeIndirectFlag = flag.Bool("includeIndirect", false, "Include indirect dependencies")
	licenceDataFlag     = flag.String("licenceData", "licence.db", "Path to the licence database")
	noticeTemplateFlag  = flag.String("noticeTemplate", "templates/NOTICE.txt.tmpl", "Path to the NOTICE template file")
	noticeOutFlag       = flag.String("noticeOut", "NOTICE.txt", "Path to output the notice")
	overridesFlag       = flag.String("overrides", "", "Path to the file containing override directives")
	validateFlag        = flag.Bool("validate", false, "Validate results (slow)")
)

func main() {
	flag.Parse()

	// create reader for dependency information
	depInput, err := mkReader(*inFlag)
	if err != nil {
		log.Fatalf("Failed to create reader for %s: %v", *inFlag, err)
	}
	defer depInput.Close()

	// create licence classifier
	classifier, err := detector.NewClassifier(*licenceDataFlag)
	if err != nil {
		log.Fatalf("Failed to create licence classifier: %v", err)
	}

	// load overrides
	overrides, err := dependency.LoadOverrides(*overridesFlag)
	if err != nil {
		log.Fatalf("Failed to load overrides: %v", err)
	}

	dependencies, err := detector.Detect(depInput, classifier, overrides, *includeIndirectFlag)
	if err != nil {
		log.Fatalf("Failed to detect licences: %v", err)
	}

	if *validateFlag {
		if err := validate.Validate(dependencies); err != nil {
			log.Fatalf("Validation failed: %v", err)
		}
	}

	if err := render.Template(dependencies, *noticeTemplateFlag, *noticeOutFlag); err != nil {
		log.Fatalf("Failed to render notice: %v", err)
	}

	if err := render.Template(dependencies, *depsTemplateFlag, *depsOutFlag); err != nil {
		log.Fatalf("Failed to render dependency list: %v", err)
	}
}

func mkReader(path string) (io.ReadCloser, error) {
	if path == "-" {
		return ioutil.NopCloser(os.Stdin), nil
	}

	return os.Open(path)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	depFile = flag.String("f", "modules.txt", "File with the list of dependencies")
	dir     = flag.String("d", "", "Project directory")
)

type Dependency struct {
	Name    string
	Version string
	License string
}

type Dependencies struct {
	List []*Dependency
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	deps, err := loadFile(filepath.Join(*dir, "vendor", *depFile))
	if err != nil {
		log.Fatalf("Can't open file with dependencies: %s", err.Error())
	}

	issues := checkForLicense(deps)
	if issues > 0 {
		log.Fatal("Can't create NOTICE.txt, there are issues with dependencies!")
	}

	createNoticeFile(deps)
}

func loadFile(path string) (*Dependencies, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	deps := &Dependencies{}
	for _, v := range lines {
		if strings.HasPrefix(v, "#") {
			dep := strings.Split(v, " ")
			deps.List = append(deps.List, &Dependency{Name: dep[1], Version: dep[2]})
		}
	}

	return deps, nil
}

func checkForLicense(deps *Dependencies) int {
	var issues []string
	licenses := []string{"LICENSE", "LICENSE.txt", "LICENCE"} // Used to keep all possible names of license files
	for _, dep := range deps.List {
		counter := len(licenses)
		for _, v := range licenses {
			bytes, err := ioutil.ReadFile(filepath.Join(*dir, "vendor", dep.Name, v))
			if err != nil {
				counter--
				continue
			}
			dep.License = string(bytes)
			break
		}
		if counter == 0 {
			issues = append(issues, fmt.Sprintf("Can't find file with license for %s version %s", dep.Name, dep.Version))
		}
	}

	if len(issues) > 0 {
		fmt.Println("Number of issues:", len(issues))
		for _, v := range issues {
			fmt.Println(v)
		}
	}

	return len(issues)
}

func createNoticeFile(deps *Dependencies) {
	var tmpl = `Elastic Cloud on Kubernetes
Copyright 2014-2019 Elasticsearch BV

This product includes software developed by The Apache Software 
Foundation (http://www.apache.org/).

==========================================================================
Third party libraries used by the Elastic Cloud on Kubernetes project:
==========================================================================

{{range $i,$v := .}} 
Dependency: {{ $v.Name }}
Version: {{ $v.Version }}

{{ $v.License }}
--------------------------------------------------------------------------
{{end}}
`
	f, err := os.Create(filepath.Join(*dir, "NOTICE.txt"))
	if err != nil {
		log.Fatalf("Can't create NOTICE.txt: %s", err.Error())
	}

	t := template.Must(template.New("notice").Parse(tmpl))
	err = t.Execute(f, deps.List)
	if err != nil {
		log.Fatalf("Failed on creating list of licenses for dependencies: %s", err.Error())
	}

	fmt.Println("NOTICE.txt was generated!")
}

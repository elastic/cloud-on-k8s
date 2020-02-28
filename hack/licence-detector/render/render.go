// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package render

import (
	"bytes"
	"fmt"
	"go/build"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/dependency"
)

var goModCache = filepath.Join(build.Default.GOPATH, "pkg", "mod")

func Template(dependencies *dependency.List, templatePath, outputPath string) error {
	funcMap := template.FuncMap{
		"currentYear": CurrentYear,
		"line":        Line,
		"licenceText": LicenceText,
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(funcMap).ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template at %s: %w", templatePath, err)
	}

	w, cleanup, err := mkWriter(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	defer cleanup()

	if err := tmpl.Execute(w, dependencies); err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	return nil
}

func mkWriter(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}

	f, err := os.Create(path)
	return f, func() { f.Close() }, err
}

/* Template functions */

func CurrentYear() string {
	return strconv.Itoa(time.Now().Year())
}

func Line(ch string) string {
	return strings.Repeat(ch, 80)
}

func LicenceText(depInfo dependency.Info) string {
	if depInfo.LicenceFile == "" {
		return "No licence file provided."
	}

	var buf bytes.Buffer
	if depInfo.LicenceTextOverrideFile != "" {
		buf.WriteString("Contents of provided licence file")
	} else {
		buf.WriteString("Contents of probable licence file ")
		buf.WriteString(strings.Replace(depInfo.LicenceFile, goModCache, "$GOMODCACHE", -1))
	}
	buf.WriteString(":\n\n")

	f, err := os.Open(depInfo.LicenceFile)
	if err != nil {
		log.Fatalf("Failed to open licence file %s: %v", depInfo.LicenceFile, err)
	}
	defer f.Close()

	_, err = io.Copy(&buf, f)
	if err != nil {
		log.Fatalf("Failed to read licence file %s: %v", depInfo.LicenceFile, err)
	}

	return buf.String()
}

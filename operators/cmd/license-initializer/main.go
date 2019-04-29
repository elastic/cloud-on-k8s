// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
)

func main() {
	var outFile string
	flag.StringVar(&outFile, "out", "", "file to write generated output to")
	flag.Parse()
	const pubKeyEnvVar = "LICENSE_PUBKEY"
	pubkeyFile := os.Getenv(pubKeyEnvVar)
	if pubkeyFile == "" {
		handleErr(fmt.Errorf("%s is a required environment variable pointing to a DER encoded public key", pubKeyEnvVar))
	}
	var out io.Writer
	if outFile == "" {
		out = os.Stdout
	} else {
		file, err := os.Create(outFile)
		if err != nil {
			handleErr(err)
		}
		defer file.Close()
		out = file
	}

	generateSrc(pubkeyFile, out)
}

func generateSrc(pubkeyFile string, out io.Writer) {
	type params struct {
		Bytes       []byte
		ShouldBreak func(int) bool
	}
	var tmpl = `// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license 

var publicKeyBytes = []byte{
{{ $break := .ShouldBreak }}
{{- range $index, $elem := .Bytes -}}
{{printf "0x%02x," $elem}}{{if (call $break $index)}}
{{end}}
{{- end}}
}
`

	bytes, err := ioutil.ReadFile(pubkeyFile)
	if err != nil {
		handleErr(errors.Wrapf(err, "Failed to read %v", pubkeyFile))
	}
	t := template.Must(template.New("license").Parse(tmpl))
	err = t.Execute(out, params{
		Bytes: bytes,
		ShouldBreak: func(i int) bool {
			return (i+1)%8 == 0
		},
	})
	if err != nil {
		handleErr(errors.Wrap(err, "Failed to write template"))
	}
}

func handleErr(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}

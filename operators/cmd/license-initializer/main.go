// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	pubKeyFlag  = "license-pubkey"
	outFileFlag = "out"
)

var Cmd = &cobra.Command{
	Use:   "license-initializer",
	Short: "Embed license public key in source",
	Run: func(cmd *cobra.Command, args []string) {
		pubkeyFile := viper.GetString(pubKeyFlag)
		if pubkeyFile == "" {
			handleErr(fmt.Errorf("%s is a required environment variable pointing to a DER encoded public key", pubKeyFlag))
		}
		outFile := viper.GetString(outFileFlag)
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
	},
}

func init() {
	Cmd.Flags().String(
		pubKeyFlag,
		"",
		"path pointing to a DER encoded public key",
	)
	Cmd.Flags().String(
		outFileFlag,
		"",
		"file to write generated output to",
	)
	handleErr(viper.BindPFlags(Cmd.Flags()))
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func main() {
	handleErr(Cmd.Execute())
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

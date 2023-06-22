// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
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

		c := vault.NewClientProvider()

		bytes, err := vault.ReadFile(c, vault.SecretFile{
			Name:          pubkeyFile,
			Path:          "license",
			FieldResolver: vault.LicensePubKeyPrefix("pubkey"),
			Base64Encoded: true,
		})
		handleErr(errors.Wrapf(err, "Failed to read %v", pubkeyFile))

		outFile := viper.GetString(outFileFlag)
		var out io.Writer
		if outFile == "" {
			out = os.Stdout
		} else {
			file, err := os.Create(outFile)
			handleErr(err)

			defer file.Close()
			out = file
		}

		generateSrc(bytes, out)
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

func generateSrc(bytes []byte, out io.Writer) {
	type params struct {
		Bytes       []byte
		ShouldBreak func(int) bool
	}
	var tmpl = `// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license 

var publicKeyBytes = []byte{
{{ $break := .ShouldBreak }}
{{- range $index, $elem := .Bytes -}}
{{printf "0x%02x," $elem}}{{if (call $break $index)}}
{{end}}
{{- end}}
}
`
	t := template.Must(template.New("license").Parse(tmpl))
	err := t.Execute(out, params{
		Bytes: bytes,
		ShouldBreak: func(i int) bool {
			return (i+1)%8 == 0
		},
	})
	handleErr(errors.Wrap(err, "Failed to write template"))
}

func handleErr(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}

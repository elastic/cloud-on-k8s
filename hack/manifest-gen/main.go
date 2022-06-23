// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/manifest-gen/internal"
)

var (
	generateFlags = internal.GenerateFlags{}
	optionsFlags  = internal.OptionsFlags{}
	sourceFlag    string
)

func main() {
	cmd := buildCmd()
	if err := cmd.Execute(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func buildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "manifest-gen",
		Short:         "ECK manifest generator",
		Long:          `Generates an installation manifest for ECK`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	sourceDir, err := findSourceDir()
	if err != nil {
		log.Fatalf("Failed to locate assets: %v", err)
	}

	cmd.PersistentFlags().StringVar(&sourceFlag, "source", sourceDir, "Source directory")

	cmd.AddCommand(generateCmd())
	cmd.AddCommand(optionsCmd())

	return cmd
}

func findSourceDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	path, err := filepath.Abs(execPath)
	if err != nil {
		return "", err
	}

	return filepath.Clean(filepath.Join(filepath.Dir(path), "..", "..", "deploy", "eck-operator")), nil
}

func generateCmd() *cobra.Command {
	desc := `
Generates a manifest for installing ECK.

There are two pre-defined profiles for installing ECK. The "global" profile installs ECK with access to the
whole Kubernetes cluster. The "restricted" profile installs ECK restricted to a single (or several) namespace(s).

The generated manifest can be customized using the "--set" and "--values" flags. Use the "options" command to list all
available configuration options. The behaviour of these flags is identical to the similarly named flags used by Helm.
See https://helm.sh/docs/intro/using_helm/ for more information.

By default, the operator is installed into the "elastic-system" namespace. This can be overridden by setting the
"operator.namespace" option.

`
	examples := `
Global operator:
    $ manifest-gen generate

Global operator with the validation webhook disabled:
    $ manifest-gen generate --set=webhook.enabled=false

Global operator with resource memory limit increased to 300Mi and CPU limit increased to 2:
    $ manifest-gen generate --set=resources.limits.cpu=2,resources.limits.memory=300Mi

Restricted operator without CRDs, managing the "elastic-system" namespace:
    $ manifest-gen generate --profile=restricted --set=installCRDs=false

Restricted operator installed to and managing the single namespace named "namespacex":
    $ manifest-gen generate --profile=restricted --namespace=namespacex --set=managedNamespaces='{namespacex}'

Restricted operator managing "elastic-system", "nsa" and "nsb":
    $ manifest-gen generate --profile=restricted --set=managedNamespaces='{elastic-system, nsa, nsb}'

Restricted operator with tracing configured:
    $ manifest-gen generate --profile=restricted --set=tracing.enabled=true --set=tracing.config.ELASTIC_APM_SERVER_URL=http://apm:8200
`

	cmd := &cobra.Command{
		Use:           "generate",
		Short:         "Generate ECK manifests",
		Long:          desc,
		Example:       examples,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			generateFlags.Source = sourceFlag
			return internal.Generate(&generateFlags)
		},
	}

	cmd.Flags().StringVar(&generateFlags.Profile, "profile", "", "Operator profile (global, restricted)")
	cmd.Flags().StringArrayVar(&generateFlags.Values, "set", []string{}, "Set additional options")
	cmd.Flags().StringArrayVar(&generateFlags.ValueFiles, "values", []string{}, "Set additional options from file(s)")
	cmd.Flags().StringVarP(&generateFlags.OperatorNamespace, "namespace", "n", "elastic-system", "Operator namespace")

	return cmd
}

func optionsCmd() *cobra.Command {
	desc := `
Displays the available options for customizing the generated manifest.

The options listed can be passed to the "generate" command using the "--set" flag.
`
	cmd := &cobra.Command{
		Use:           "options",
		Short:         "List manifest options",
		Long:          desc,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			optionsFlags.Source = sourceFlag
			return internal.Options(&optionsFlags)
		},
	}

	return cmd
}

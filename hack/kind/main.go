// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// manifest is a kind cluster template
// the explicit podSubnet definition can be removed as soon as https://github.com/kubernetes-sigs/kind/commit/60074a9e67ddc8d35d3468ab137358b62a4cf723
// will be available in a released version of kind
const manifest = `---
kind: Cluster
apiVersion: {{.APIVersion}}
networking:
  ipFamily: {{.IPFamily}}
{{- if eq .IPFamily "ipv6" }}
  podSubnet: "fd00:10:244::/56"
{{- end}}
nodes:
  - role: control-plane
{{- range .WorkerNames }}
  - role: worker
{{- end}}
`

var k kind

type startArgs struct {
	IPFamily     string
	skipSetup    bool
	imagesToLoad []string
	nodes        int
	nodeImage    string
}

func (args startArgs) WorkerNames() []string {
	var names []string
	// kind has the following naming scheme <cluster-name>-worker, <cluster-name>-worker2 etc
	// this is not configurable and thus explains the awkward name construction here
	for i := 0; i < args.nodes; i++ {
		suffix := ""
		if i > 0 {
			suffix = strconv.Itoa(i + 1)
		}
		names = append(names, fmt.Sprintf("%s-worker%s", k.clusterName, suffix))
	}
	return names
}

func main() {
	cmd := &cobra.Command{
		Use:   "setup-kind",
		Short: "Kind test tooling",
		Long:  "Setup and tear down of kind clusters for testing",
	}
	cmd.PersistentFlags().StringVar(&k.clusterName, "cluster-name", "eck-e2e", "Kind cluster name")
	cmd.PersistentFlags().StringVar(&k.kindVersion, "kind-version", "0.9.0", "Kind version to use (currently supported on CI: 0.8.1, 0.9.0)")
	cmd.PersistentFlags().IntVarP(&k.logLevel, "verbosity", "v", 0, "Kind log verbosity")

	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		// check that the kind executable is installed in the right version
		binaryName := "kind-" + k.kindVersion
		path, err := exec.LookPath(binaryName)
		if err != nil {
			return err
		}
		// populate the kind struct with version specific information
		k.binary = path
		k.apiVersion = "kind.sigs.k8s.io/v1alpha3"
		v := semver.MustParse(k.kindVersion)
		if v.GTE(semver.MustParse("0.9.0")) {
			k.apiVersion = "kind.x-k8s.io/v1alpha4"
		}
		return nil
	}

	cmd.AddCommand(startCmd(), stopCmd())
	if err := cmd.Execute(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func stopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a kind k8s cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return k.stop()
		},
	}
	return cmd
}

func startCmd() *cobra.Command {
	var startArgs startArgs
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a kind k8s cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !startArgs.skipSetup {
				if err := k.setup(startArgs); err != nil {
					return err
				}
			}

			if err := k.loadImages(startArgs); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&startArgs.IPFamily, "ip-family", "ipv4", "IP family (ipv4, ipv6)")
	cmd.Flags().BoolVar(&startArgs.skipSetup, "skip-setup", false, "Skip creation of kind cluster")
	cmd.Flags().StringArrayVar(&startArgs.imagesToLoad, "load-image", []string{}, "Docker image to load into the kind cluster that can't or should not be loaded from a remote registry")
	cmd.Flags().IntVar(&startArgs.nodes, "nodes", 3, "How many k8s worker nodes to create")
	cmd.Flags().StringVar(&startArgs.nodeImage, "node-image", "kindest/node:v1.20.0", "Kind node image to use. Must be compatible with kind-version")
	return cmd
}

type kind struct {
	binary      string
	kindVersion string
	apiVersion  string
	logLevel    int
	clusterName string
}

func (k kind) cmd(args ...string) *exec.Cmd {
	effectiveArgs := []string{"-v", strconv.Itoa(k.logLevel)}
	effectiveArgs = append(effectiveArgs, args...)
	effectiveArgs = append(effectiveArgs, "--name", k.clusterName)

	cmd := exec.Command(k.binary, effectiveArgs...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (k kind) loadImages(args startArgs) error {
	for _, image := range args.imagesToLoad {
		err := k.cmd("load", "docker-image", "--nodes", strings.Join(args.WorkerNames(), ","), image).Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func (k kind) createTmpManifest(args startArgs) (*os.File, error) {
	f, err := ioutil.TempFile("", "kind-cluster")
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("cluster.yaml").Parse(manifest)
	if err != nil {
		return nil, err
	}

	type tplArgs struct {
		startArgs
		APIVersion string
	}
	ta := tplArgs{
		startArgs:  args,
		APIVersion: k.apiVersion,
	}
	return f, tmpl.Execute(f, ta)
}

func (k kind) stop() error {
	return k.cmd("delete", "cluster").Run()
}

func (k kind) setup(args startArgs) error {
	// Write manifest to temporary file
	tmpManifest, err := k.createTmpManifest(args)
	if err != nil {
		return err
	}
	defer os.Remove(tmpManifest.Name())

	// Delete any previous e2e kind cluster with the same name
	err = k.stop()
	if err != nil {
		return err
	}

	err = k.cmd("create", "cluster", "--config", tmpManifest.Name(), "--retain", "--image", args.nodeImage).Run()
	if err != nil {
		return err
	}

	// Get kubeconfig from kind
	cmd := k.cmd("get", "kubeconfig")
	cmd.Stdout = nil // we want to override out
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Persist kubeconfig for reliabililty in following kubectl commands
	kubeCfg, err := ioutil.TempFile("", "kubeconfig")
	if err != nil {
		return err
	}
	defer os.Remove(kubeCfg.Name())
	_, err = kubeCfg.Write(output)
	if err != nil {
		return err
	}

	// Delete standard storage class but ignore error if not found
	if err := kubectl("--kubeconfig", kubeCfg.Name(), "delete", "storageclass", "standard"); err != nil {
		println(err.Error())
		return err
	}

	wd, err := scriptWd()
	if err != nil {
		return err
	}

	if err := kubectl("--kubeconfig", kubeCfg.Name(), "apply", "-f", filepath.Join(wd, "storageclass.yaml")); err != nil {
		return err
	}
	return nil
}

func kubectl(arg ...string) error {
	output, err := exec.Command("kubectl", arg...).CombinedOutput()
	fmt.Println(string(output))
	if err != nil && strings.Contains(string(output), "Error from server (NotFound)") {
		fmt.Printf("Ignoring NotFound error for command: %v\n", arg)
		return nil //ignore not found errors
	}
	return err
}

func scriptWd() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not detect working directory")
	}
	return filepath.Dir(file), nil
}

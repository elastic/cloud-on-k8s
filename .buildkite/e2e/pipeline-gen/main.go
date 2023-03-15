// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"

	"log"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

var (
	//go:embed pipeline.tpl.yaml
	pipelineTemplate string

	// providersInDocker are k8s providers that require the deployer to run in Docker
	providersInDocker = []string{"kind", "aks", "ocp", "tanzu"}
	// providersNoCleanup are k8s providers that do not require the cluster to be deleted after use
	providersNoCleanup = []string{"kind"}

	semverRE = regexp.MustCompile(`\d*\.\d*\.\d*(-\w*)?`)
	chars    = []rune("0123456789abcdefghijklmnopqrstuvwxyz")

	fixed string
	mixed string

	output string
)

func init() {
	flag.StringVar(&fixed, "f", "", "fixed variables")
	flag.StringVar(&mixed, "m", "", "mixed variables")
	flag.StringVar(&output, "o", "buildkite-pipeline", "Type of output: buildkite-pipeline or envfile")
	flag.Parse()

	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
	stat, err := os.Stdin.Stat()
	handlErr("failed to read stdin", err)

	var runs []Runs

	// no stdin
	if stat.Mode()&os.ModeCharDevice != 0 {
		if output == "envfile" {
			for k, v := range commonTestEnv("dev", "dev") {
				fmt.Printf("%s=%s\n", k, v)
			}
			return
		}

		fixedEnv, err := listToEnv(fixed)
		handlErr("failed to read fixed variables", err)

		mixedEnv, err := listToEnvs(mixed)
		handlErr("failed to read mixed variables", err)

		runs = []Runs{{
			Fixed: fixedEnv,
			Mixed: mixedEnv,
		}}

	} else {
		in, err := io.ReadAll(os.Stdin)
		handlErr("failed to read stdin", err)
		if len(in) == 0 {
			handlErr("failed to read stdin", errors.New("nothing on /dev/stdin"))
		}

		err = yaml.Unmarshal(in, &runs)
		handlErr("failed to parse stdin", err)
	}

	// build a flat list of the tests to run
	tests := make([]TestRun, 0)
	cleanup := false
	for i := range runs {
		if runs[i].Mixed == nil {
			runs[i].Mixed = []Env{{}}
		}
		for j := range runs[i].Mixed {
			test, err := newTest(runs[i].Label, runs[i].Fixed, runs[i].Mixed[j])
			handlErr("failed to create new test", err)

			tests = append(tests, test)
			cleanup = cleanup || test.Cleanup
		}
	}

	tpl, err := template.New("pipeline.yaml").Parse(pipelineTemplate)
	handlErr("failed to parse template", err)

	err = tpl.Execute(os.Stdout, map[string]interface{}{
		"Cleanup": cleanup,
		"Tests":   tests,
	})
	handlErr("failed to generate pipeline", err)
}

func listToEnv(str string) (Env, error) {
	env := Env{}
	for _, elem := range strings.Split(str, ",") {
		kv := strings.Split(elem, "=")
		if len(kv) != 2 {
			return nil, fmt.Errorf("no environment variable found in format `k=v` for %s", kv)
		}
		env[kv[0]] = kv[1]
	}
	return env, nil
}

func listToEnvs(str string) ([]Env, error) {
	envs := []Env{}
	for _, elem := range strings.Split(str, ",") {
		kv := strings.Split(elem, "=")
		if len(kv) != 2 {
			return nil, fmt.Errorf("no environment variable found in format `k=v` for %s", kv)
		}
		envs = append(envs, Env{kv[0]: kv[1]})
	}
	return envs, nil
}

type Runs struct {
	Label string
	Fixed Env
	Mixed []Env
}

// Env corresponds to the environment variables to run a test
type Env map[string]string

// TestRun is a run of the full e2e tests suite
type TestRun struct {
	Name     string
	SlugName string
	Env      Env
	Dind     bool
	Cleanup  bool
}

func newTest(parentLabel string, fixed Env, mixed Env) (TestRun, error) {
	provider, ok := fixed["E2E_PROVIDER"]
	if !ok {
		return TestRun{}, fmt.Errorf("E2E_PROVIDER not defined")
	}

	name := parentLabel
	if name == "" {
		name = provider
	}

	// use the two first env values as suffix if more than one env in the matrix
	// to name the test
	if len(mixed) > 1 {
		suffixes := make([]string, 0)
		i := 0
		for _, val := range mixed {
			suffix := val
			// extract semver (e.g.: kind node image)
			match := semverRE.FindStringSubmatch(suffix)
			if len(match) > 0 {
				suffix = match[0]
			}
			suffixes = append(suffixes, suffix)
			i++
			if i == 2 {
				break
			}
		}
		sort.Strings(suffixes)
		name = fmt.Sprintf("%s-%s", name, strings.Join(suffixes, "-"))
	}

	slugName := fmt.Sprintf("%s-%s", truncateText(sanitize(name), 16), randString(4))

	t := TestRun{
		Name:     name,
		SlugName: slugName,
		Dind:     slices.Contains(providersInDocker, provider),
		Cleanup:  !slices.Contains(providersNoCleanup, provider),
		Env:      commonTestEnv(name, slugName),
	}

	for k, v := range fixed {
		t.Env[k] = v
	}
	for k, v := range mixed {
		t.Env[k] = v
	}

	return t, nil
}

func commonTestEnv(name string, slugName string) map[string]string {
	buildN, ok := os.LookupEnv("BUILDKITE_BUILD_NUMBER")
	if !ok {
		buildN = "0"
	}

	return map[string]string{
		"PIPELINE":              fmt.Sprintf("e2e/%s", name),
		"CLUSTER_NAME":          fmt.Sprintf("eck-e2e-%s-%s", slugName, buildN),
		"BUILD_NUMBER":          buildN,
		"TEST_OPTS":             "-race",
		"E2E_JSON":              "true",
		"GO_TAGS":               "release",
		"export LICENSE_PUBKEY": "in-memory",
		"TEST_LICENSE":          "in-memory",
		"MONITORING_SECRETS":    "in-memory",
		"OPERATOR_IMAGE":        getMetadata("operator-image") + operatorImageSuffix(),
		"E2E_IMG":               getMetadata("e2e-image"),
	}
}

func operatorImageSuffix() string {
	suffix := os.Getenv("BUILD_LICENSE_PUBKEY")
	if suffix != "" {
		return fmt.Sprintf("-%s", suffix)
	}
	return suffix
}

func getMetadata(key string) string {
	var cmd *exec.Cmd
	if os.Getenv("CI") == "true" {
		cmd = exec.Command("buildkite-agent", "meta-data", "get", key)
	} else {
		// dev mode
		return "TO BE SET"
	}
	out, err := cmd.Output()
	if err != nil {
		log.Fatal("fail to execute: ", cmd, err)
	}
	return strings.Trim(string(out), "\n")
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "/", "-")
	return strings.ToLower(s)
}

func truncateText(s string, max int) string {
	if max > len(s) {
		return s
	}
	return s[:max]
}

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func handlErr(context string, err error) {
	if err != nil {
		fmt.Printf("%s: %s\n", context, err)
		os.Exit(1)
	}
}

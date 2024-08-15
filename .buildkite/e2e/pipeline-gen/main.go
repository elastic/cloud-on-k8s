// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"text/template"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

const (
	EnvVarProvider             = "E2E_PROVIDER"
	EnvVarK8sVersion           = "DEPLOYER_K8S_VERSION"
	EnvVarStackVersion         = "E2E_STACK_VERSION"
	EnvVarBuildkiteBuildNumber = "BUILDKITE_BUILD_NUMBER"
	EnvVarBuildNumber          = "BUILD_NUMBER"
	EnvVarPipeline             = "PIPELINE"
	EnvVarClusterName          = "CLUSTER_NAME"
	EnvVarTestOpts             = "TEST_OPTS"
	EnvVarTestsMatch           = "TESTS_MATCH"
	EnvVarTestLicensePKeyPath  = "TEST_LICENSE_PKEY_PATH"
	EnvVarBuildLicensePubkey   = "BUILD_LICENSE_PUBKEY"
	EnvVarLicensePubKey        = "export LICENSE_PUBKEY"
	EnvVarTestLicense          = "TEST_LICENSE"
	EnvVarMonitoringSecrets    = "MONITORING_SECRETS"
	EnvVarE2EJson              = "E2E_JSON"
	EnvVarGoTags               = "GO_TAGS"
	EnvVarOperatorImage        = "OPERATOR_IMAGE"
	EnvVarE2EImage             = "E2E_IMG"

	KindAgentsMachineType = "n1-standard-16"
)

var (
	//go:embed pipeline.tpl.yaml
	pipelineTemplate string

	// providersInDocker are k8s providers that require the deployer to run in Docker
	providersInDocker = []string{"kind", "aks", "ocp"}
	// providersNoCleanup are k8s providers that do not require the cluster to be deleted after use
	providersNoCleanup = []string{"kind"}
	// providers are k8s providers for which it is not possible to retrieve the kube config after cluster creation
	providersNoRemoteConfig = []string{"kind"}

	semverRE = regexp.MustCompile(`\d*\.\d*\.\d*(-\w*)?`)
	chars    = []rune("abcdefghijklmnopqrstuvwxyz")

	shortcuts = map[string]string{
		"p": EnvVarProvider,
		"k": EnvVarK8sVersion,
		"s": EnvVarStackVersion,
		"t": EnvVarTestsMatch,
	}

	fixed   string
	mixed   string
	envFile bool

	rootDir string
)

func init() {
	flag.StringVar(&fixed, "f", "", "Fixed variables (comma-separated list)")
	flag.StringVar(&mixed, "m", "", "Mixed variables (comma-separated list of circumflex-separated list)")
	flag.BoolVar(&envFile, "e", false, "Output in .env file format. Supports only one test.")
	flag.Parse()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Printf("failed to get current path")
		os.Exit(1)
	}
	rootDir = filepath.Join(filepath.Dir(filename), "../../..")
}

func main() {
	stat, err := os.Stdin.Stat()
	handleErr("Failed to read stdin", err)

	var groups []Group

	// no stdin
	if stat.Mode()&os.ModeCharDevice != 0 {

		fixedEnv, err := stringListToEnv(fixed)
		handleErr("Failed to read fixed variables", err)

		mixedEnv, err := stringListToEnvs(mixed)
		handleErr("Failed to read mixed variables", err)

		groups = []Group{{
			Fixed: fixedEnv,
			Mixed: mixedEnv,
		}}

	} else {
		if fixed != "" {
			handleErr("Failed to read stdin", errors.New("incompatible with the --fixed flag"))
		}
		if mixed != "" {
			handleErr("Failed to read stdin", errors.New("incompatible with the --mixed flag"))
		}

		in, err := io.ReadAll(os.Stdin)
		handleErr("Failed to read stdin", err)
		if len(in) == 0 {
			handleErr("Failed to read stdin", errors.New("nothing on /dev/stdin"))
		}

		err = yaml.Unmarshal(in, &groups)
		handleErr("Failed to parse stdin", err)
	}

	// build a flat list of the tests to run
	tests := make([]TestsSuiteRun, 0)
	cleanup := false
	for i := range groups {
		if groups[i].Mixed == nil {
			groups[i].Mixed = []Env{{}}
		}
		for j := range groups[i].Mixed {
			ts, err := newTestsSuite(groups[i].Label, groups[i].Fixed, len(groups[i].Mixed), groups[i].Mixed[j])
			handleErr("Failed to create tests suite", err)

			tests = append(tests, ts)
			cleanup = cleanup || ts.Cleanup
		}
	}

	if envFile {
		if len(tests) > 1 {
			handleErr("Not supported with output in .env file format", errors.New("more than 1 test suite to run"))
			return
		}
		for k, v := range tests[0].Env {
			fmt.Printf("%s=%s\n", k, v)
		}
		return
	}

	tpl, err := template.New("pipeline.yaml").Parse(pipelineTemplate)
	handleErr("Failed to parse template", err)

	err = tpl.Execute(os.Stdout, map[string]interface{}{
		"Cleanup":               cleanup,
		"Tests":                 tests,
		"KindAgentsMachineType": KindAgentsMachineType,
	})
	handleErr("Failed to generate pipeline", err)
}

func stringListToEnv(str string) (Env, error) {
	if str == "" {
		return nil, nil
	}
	env := Env{}
	for _, elem := range strings.Split(str, ",") {
		kv := strings.Split(elem, "=")
		if len(kv) != 2 {
			return nil, fmt.Errorf("no environment variable found in format `k=v` for %s", elem)
		}
		env[kv[0]] = kv[1]
	}
	return env, nil
}

func stringListToEnvs(str string) ([]Env, error) {
	if str == "" {
		return nil, nil
	}
	var envs []Env
	mixedsStr := strings.Split(str, ",")
	for _, mixedStr := range mixedsStr {
		mixedEnv := Env{}
		envStr := strings.Split(mixedStr, "^")
		for _, kvStr := range envStr {
			kv := strings.Split(kvStr, "=")
			if len(kv) != 2 {
				return nil, fmt.Errorf("no environment variable found in format `k=v` for %s", kvStr)
			}
			mixedEnv[kv[0]] = kv[1]

		}
		envs = append(envs, mixedEnv)
	}
	return envs, nil
}

// Group corresponds to a group of e2e tests suite
type Group struct {
	Label string
	Fixed Env
	Mixed []Env
}

// Env corresponds to the environment variables to run an e2e tests suite
type Env map[string]string

// TestsSuiteRun is a run of the e2e tests suite
type TestsSuiteRun struct {
	Name             string
	Provider         string
	SlugName         string
	Env              Env
	Dind             bool
	Cleanup          bool
	RemoteKubeconfig bool
}

func newTestsSuite(groupLabel string, fixed Env, mixedLen int, mixed Env) (TestsSuiteRun, error) {
	resolveShortcuts(fixed)
	resolveShortcuts(mixed)

	// find k8s provider
	provider, ok := lookupEnv(EnvVarProvider, fixed, mixed)
	if !ok {
		return TestsSuiteRun{}, fmt.Errorf("%s not defined", EnvVarProvider)
	}

	stackVersion, _ := lookupEnv(EnvVarStackVersion, fixed, mixed)

	name := getName(groupLabel, provider, mixedLen, mixed)
	slugName := getSlugName(name)
	env, err := commonTestEnv(name, slugName, stackVersion)
	if err != nil {
		return TestsSuiteRun{}, err
	}

	t := TestsSuiteRun{
		Name:             name,
		Provider:         provider,
		SlugName:         slugName,
		Dind:             slices.Contains(providersInDocker, provider),
		Cleanup:          !slices.Contains(providersNoCleanup, provider),
		RemoteKubeconfig: !slices.Contains(providersNoRemoteConfig, provider),
		Env:              env,
	}

	// merge fixed and mixed vars
	for k, v := range fixed {
		t.Env[k] = v
	}
	for k, v := range mixed {
		t.Env[k] = v
	}

	return t, nil
}

func lookupEnv(envVarName string, fixed Env, mixed Env) (string, bool) {
	// start searching in mixed env vars that can override fixed env vars
	envVar, ok := mixed[envVarName]
	if !ok {
		envVar, ok = fixed[envVarName]
		if !ok {
			return "", false
		}
	}
	return envVar, true
}

func resolveShortcuts(e Env) {
	for k, v := range e {
		for short, long := range shortcuts {
			if k == short {
				e[long] = v
				delete(e, short)
			}
		}
	}
}

func getName(groupLabel, provider string, mixedLen int, mixed Env) string {
	name := groupLabel
	if name == "" {
		name = provider
	}

	if mixedLen < 2 {
		return name
	}

	// try use the two first env var values as suffix if more than one var in the mixed vars

	suffixes := make([]string, 0)
	i := 0
	for _, val := range mixed {
		suffix := val
		// do not repeat name
		if suffix == name {
			continue
		}
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

	if len(suffixes) == 0 {
		return name
	}

	return fmt.Sprintf("%s-%s", name, strings.Join(suffixes, "-"))

}

func getSlugName(name string) string {
	// sanitize
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ToLower(name)

	// truncate
	if len(name) > 16 {
		name = name[:16]
	}

	// random id
	id := make([]rune, 4)
	for i := range id {
		id[i] = chars[rand.Intn(len(chars))]
	}

	return fmt.Sprintf("%s-%s", name, string(id))
}

func commonTestEnv(name string, slugName string, stackVersion string) (map[string]string, error) {
	buildNumber, ok := os.LookupEnv(EnvVarBuildkiteBuildNumber)
	if !ok {
		buildNumber = "0"
	}

	operatorImage, err := getMetadata("operator-image")
	if err != nil {
		return nil, fmt.Errorf("unable to get operator-image metadata: %w", err)
	}
	e2eImage, err := getMetadata("e2e-image")
	if err != nil {
		return nil, fmt.Errorf("unable to get e2e-image metadata: %w", err)
	}

	env := map[string]string{
		EnvVarPipeline:          fmt.Sprintf("e2e/%s", name),
		EnvVarClusterName:       fmt.Sprintf("eck-e2e-%s-%s", slugName, buildNumber),
		EnvVarBuildNumber:       buildNumber,
		EnvVarTestOpts:          "-race",
		EnvVarE2EJson:           "true",
		EnvVarGoTags:            "release",
		EnvVarLicensePubKey:     "in-memory",
		EnvVarTestLicense:       "in-memory",
		EnvVarMonitoringSecrets: "in-memory",
		EnvVarOperatorImage:     operatorImage,
		EnvVarE2EImage:          e2eImage,
	}

	// automatically add special vars to test snapshot stack versions
	if strings.HasSuffix(stackVersion, "-SNAPSHOT") {
		// dev operator image build
		env[EnvVarOperatorImage] = fmt.Sprintf("%s-%s", env[EnvVarOperatorImage], "dev")
		// dev public key to verify test licenses
		env[EnvVarBuildLicensePubkey] = "dev"
		// dev private key to generate test licenses
		env[EnvVarTestLicensePKeyPath] = "in-memory"
	}

	// dev mode
	if os.Getenv("CI") != "true" {
		env[EnvVarLicensePubKey] = filepath.Join(rootDir, ".ci/license.key")
		env[EnvVarTestLicense] = filepath.Join(rootDir, ".ci/test-license.json")
		env[EnvVarMonitoringSecrets] = ""
	}

	return env, nil
}

func getMetadata(key string) (string, error) {
	var cmd *exec.Cmd
	if os.Getenv("CI") == "true" {
		cmd = exec.Command("buildkite-agent", "meta-data", "get", key)
	} else {
		cmd = exec.Command("make", "-C", rootDir, fmt.Sprintf("print-%s", key))
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.Trim(string(out), "\n"), nil
}

func handleErr(context string, err error) {
	if err != nil {
		fmt.Printf("%s: %s\n", context, err)
		os.Exit(1)
	}
}

package elasticsearch

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
)

type Comparison struct {
	Match           bool
	MismatchReasons []string
}

func NewComparison(match bool, mismatchReasons ...string) Comparison {
	return Comparison{Match: match, MismatchReasons: mismatchReasons}
}

var ComparisonMatch = NewComparison(true)

func ComparisonMismatch(mismatchReasons ...string) Comparison {
	return NewComparison(false, mismatchReasons...)
}

func NewStringComparison(expected string, actual string, name string) Comparison {
	return NewComparison(expected == actual, fmt.Sprintf("%s mismatch: expected %s, actual %s", name, expected, actual))
}

// getEsContainer returns the elasticsearch container in the given pod
func getEsContainer(containers []corev1.Container) (corev1.Container, error) {
	for _, c := range containers {
		if c.Name == containerName {
			return c, nil
		}
	}
	return corev1.Container{}, fmt.Errorf("No container named %s in the given pod", containerName)
}

// envVarsByName turns the given list of env vars into a map: EnvVar.Name -> EnvVar
func envVarsByName(vars []corev1.EnvVar) map[string]corev1.EnvVar {
	m := make(map[string]corev1.EnvVar, len(vars))
	for _, v := range vars {
		m[v.Name] = v
	}
	return m
}

// compareEnvironmentVariables returns true if the given env vars can be considered equal
// Note that it does not compare referenced values (eg. from secrets)
func compareEnvironmentVariables(actual []corev1.EnvVar, expected []corev1.EnvVar) Comparison {
	actualByName := envVarsByName(actual)
	expectedByName := envVarsByName(expected)
	for _, v := range comparableEnvVars {
		actualVar, inActual := actualByName[v]
		expectedVar, inExpected := expectedByName[v]
		if inActual != inExpected || actualVar.Value != expectedVar.Value {
			return ComparisonMismatch(fmt.Sprintf("Environment variable %s mismatch: expected %s, actual %s", v, expectedVar.Value, actualVar.Value))
		}
	}
	return ComparisonMatch
}

// compareResources returns true if both resources match
func compareResources(actual corev1.ResourceRequirements, expected corev1.ResourceRequirements) Comparison {
	if !reflect.DeepEqual(actual.Limits, expected.Limits) {
		return ComparisonMismatch(fmt.Sprintf("Different resource limits: expected %+v, actual %+v", actual.Limits, expected.Limits))
	}
	if !reflect.DeepEqual(actual.Requests, expected.Requests) {
		return ComparisonMismatch(fmt.Sprintf("Different resource requests: expected %+v, actual %+v", actual.Requests, expected.Requests))
	}
	return ComparisonMatch
}

func podMatchesSpec(pod corev1.Pod, spec PodSpecContext) (bool, []string, error) {
	actualContainer, err := getEsContainer(pod.Spec.Containers)
	if err != nil {
		return false, nil, err
	}
	expectedContainer, err := getEsContainer(spec.PodSpec.Containers)
	if err != nil {
		return false, nil, err
	}

	// TODO: compare volume claims?

	comparisons := []Comparison{
		NewStringComparison(expectedContainer.Image, actualContainer.Image, "Docker image"),
		NewStringComparison(expectedContainer.Name, actualContainer.Name, "Container name"),
		compareEnvironmentVariables(actualContainer.Env, expectedContainer.Env),
		compareResources(actualContainer.Resources, expectedContainer.Resources),
		// Non-exhaustive list of ignored stuff:
		// - pod labels
		// - node name
		// - discovery.zen.ping.unicast.hosts
		// - cluster.name
		// - discovery.zen.minimum_master_nodes
		// - network.host
		// - probe password
		// - volume and volume mounts
		// - readiness probe
		// - termination grace period
		// - ports
		// - image pull policy
	}

	for _, c := range comparisons {
		if !c.Match {
			return false, c.MismatchReasons, nil
		}
	}

	return true, nil, nil
}

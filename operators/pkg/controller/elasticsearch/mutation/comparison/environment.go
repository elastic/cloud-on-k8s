package comparison

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

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
	actualUnmatchedByName := envVarsByName(actual)
	expectedByName := envVarsByName(expected)

	// for each expected, verify actual has a corresponding, equal (by value) entry
	for k, expectedVar := range expectedByName {
		actualVar, inActual := actualUnmatchedByName[k]
		if !inActual || actualVar.Value != expectedVar.Value {
			return ComparisonMismatch(fmt.Sprintf(
				"Environment variable %s mismatch: expected [%s], actual [%s]",
				k,
				expectedVar.Value,
				actualVar.Value,
			))
		}

		// delete from actual unmatched as it was matched
		delete(actualUnmatchedByName, k)
	}

	// if there's remaining entries in actualUnmatchedByName, it's not a match.
	if len(actualUnmatchedByName) > 0 {
		return ComparisonMismatch(fmt.Sprintf("Actual has additional env variables: %v", actualUnmatchedByName))
	}

	return ComparisonMatch
}

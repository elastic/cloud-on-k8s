package support

import (
	"fmt"
	"reflect"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

const (
	// TaintedAnnotationName used to represent a tainted resource in k8s resources
	TaintedAnnotationName = "elasticsearch.k8s.elastic.co/tainted"

	// TaintedReason message
	TaintedReason = "mismatch due to tainted node"
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
		if c.Name == DefaultContainerName {
			return c, nil
		}
	}
	return corev1.Container{}, fmt.Errorf("no container named %s in the given pod", DefaultContainerName)
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
	actualUnmatchedByName := envVarsByName(actual)
	expectedByName := envVarsByName(expected)

	// handle ignored vars do not require matching
	for _, k := range ignoredVarsDuringComparison {
		delete(actualUnmatchedByName, k)
		delete(expectedByName, k)
	}

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

// compareResources returns true if both resources match
func compareResources(actual corev1.ResourceRequirements, expected corev1.ResourceRequirements) Comparison {
	originalExpected := expected.DeepCopy()
	// need to deal with the fact actual may have defaulted values
	// we will assume for now that if expected is missing values that actual has, they will be the defaulted values
	// in effect, this will not fail a comparison if you remove limits from the spec as we cannot detect the difference
	// between a defaulted value and a missing one. moral of the story: you should definitely be explicit all the time.
	for k, v := range actual.Limits {
		if _, ok := expected.Limits[k]; !ok {
			expected.Limits[k] = v
		}
	}
	if !reflect.DeepEqual(actual.Limits, expected.Limits) {
		return ComparisonMismatch(
			fmt.Sprintf("Different resource limits: expected %+v, actual %+v", expected.Limits, actual.Limits),
		)
	}

	// If Requests is omitted for a container, it defaults to Limits if that is explicitly specified
	if len(expected.Requests) == 0 {
		expected.Requests = originalExpected.Limits
	}
	// see the discussion above re copying limits, which applies to defaulted requests as well
	for k, v := range actual.Requests {
		if _, ok := expected.Requests[k]; !ok {
			expected.Requests[k] = v
		}
	}
	if !reflect.DeepEqual(actual.Requests, expected.Requests) {
		return ComparisonMismatch(
			fmt.Sprintf("Different resource requests: expected %+v, actual %+v", expected.Requests, actual.Requests),
		)
	}
	return ComparisonMatch
}

// volumeAndPVC holds a volume and a PVC
type volumeAndPVC struct {
	volume corev1.Volume
	pvc    corev1.PersistentVolumeClaim
}

// comparePersistentVolumeClaims returns true if the expected persistent volume claims are found in the list of volumes
func comparePersistentVolumeClaims(
	actual []corev1.Volume,
	expected []corev1.PersistentVolumeClaim,
	state ResourcesState,
) Comparison {
	// TODO: handle extra PVCs that are in volumes, but not in expected claim templates

	var volumeAndPVCs []volumeAndPVC
	for _, volume := range actual {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		claimName := volume.PersistentVolumeClaim.ClaimName

		pvc, err := state.FindPVCByName(claimName)
		if err != nil {
			// this is rather unexpected, and we have two options:
			// 1. return the error to our caller (possibly changing our signature a little)
			// 2. consider the pod not matching
			// we usually expect all claims from a pod to exist, so we consider this case exceptional, but we chose to
			// go with option 2 because we'd rather see the pod be replaced than potentially getting stuck in the
			// reconciliation loop without being able to reconcile further. we also chose to log it as an error level
			// to call more attention to the fact this was occurring because we would like to try to get a better
			// understanding of the scenarios in which this may happen.
			msg := "Volume is referring to unknown PVC"
			log.Error(err, msg)
			return ComparisonMismatch(fmt.Sprintf("%s: %s", msg, err))
		}

		volumeAndPVCs = append(volumeAndPVCs, volumeAndPVC{volume: volume, pvc: pvc})
	}

ExpectedTemplates:
	for _, pvcTemplate := range expected {
		for i, actualVolumeAndPVC := range volumeAndPVCs {
			if templateMatchesActualVolumeAndPvc(pvcTemplate, actualVolumeAndPVC) {
				// remove the current from the remaining volumes so it cannot be used to match another template
				volumeAndPVCs = append(volumeAndPVCs[:i], volumeAndPVCs[i+1:]...)

				// continue the outer loop because this pvc template had a match
				continue ExpectedTemplates
			}
		}

		// at this point, we were unable to match the template with any of the volumes, so the comparison should not
		// match

		volumeNames := make([]string, len(volumeAndPVCs))
		for _, avp := range volumeAndPVCs {
			volumeNames = append(volumeNames, avp.volume.Name)
		}

		return ComparisonMismatch(fmt.Sprintf(
			"Unmatched volumeClaimTemplate: %s has no match in volumes %v",
			pvcTemplate.Name,
			volumeNames,
		))
	}

	return ComparisonMatch
}

// templateMatchesActualVolumeAndPvc returns true if the pvc matches the volumeAndPVC
func templateMatchesActualVolumeAndPvc(pvcTemplate corev1.PersistentVolumeClaim, actualVolumeAndPVC volumeAndPVC) bool {
	if pvcTemplate.Name != actualVolumeAndPVC.volume.Name {
		// name from template does not match actual, no match
		return false
	}

	// labels
	for templateLabelKey, templateLabelValue := range pvcTemplate.Labels {
		if actualValue, ok := actualVolumeAndPVC.pvc.Labels[templateLabelKey]; !ok {
			// actual is missing a key, no match
			return false
		} else if templateLabelValue != actualValue {
			// values differ, no match
			return false
		}
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.AccessModes, actualVolumeAndPVC.pvc.Spec.AccessModes) {
		return false
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.Resources, actualVolumeAndPVC.pvc.Spec.Resources) {
		return false
	}

	// this may be set to nil to be defaulted, so here we're assuming that the storage class name
	// may have been defaulted. this may cause an unintended match, which can be worked around by
	// being explicit in the pvc template spec.
	if pvcTemplate.Spec.StorageClassName != nil &&
		!reflect.DeepEqual(pvcTemplate.Spec.StorageClassName, actualVolumeAndPVC.pvc.Spec.StorageClassName) {
		return false
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.VolumeMode, actualVolumeAndPVC.pvc.Spec.VolumeMode) {
		return false
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.Selector, actualVolumeAndPVC.pvc.Spec.Selector) {
		return false
	}

	return true
}
func IsTainted(pod corev1.Pod) bool {
	if v, ok := pod.Annotations[TaintedAnnotationName]; ok {
		tainted, _ := strconv.ParseBool(v)
		return tainted
	}
	return false
}

func podMatchesSpec(pod corev1.Pod, spec PodSpecContext, state ResourcesState) (bool, []string, error) {
	actualContainer, err := getEsContainer(pod.Spec.Containers)
	if err != nil {
		return false, nil, err
	}
	expectedContainer, err := getEsContainer(spec.PodSpec.Containers)
	if err != nil {
		return false, nil, err
	}

	comparisons := []Comparison{
		NewStringComparison(expectedContainer.Image, actualContainer.Image, "Docker image"),
		NewStringComparison(expectedContainer.Name, actualContainer.Name, "Container name"),
		compareEnvironmentVariables(actualContainer.Env, expectedContainer.Env),
		compareResources(actualContainer.Resources, expectedContainer.Resources),
		comparePersistentVolumeClaims(pod.Spec.Volumes, spec.TopologySpec.VolumeClaimTemplates, state),
		// Non-exhaustive list of ignored stuff:
		// - pod labels
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

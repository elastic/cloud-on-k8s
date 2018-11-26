package pathutil

import "path"

const (
	// VolumesPath is the path into which volumes should be created
	// Must match the driver pod host mount
	// TODO: variabilize?
	VolumesPath = "/mnt/elastic-local-volumes"
	// TODO: gke needs this
	// VolumesPath = "/mnt/disks"

)

// ExtractPVCID returns the last part of the pod volume path given by kubelet,
// corresponding to the PVC ID
//
// eg. from "/var/lib/kubelet/pods/cb528df9-ecab-11e8-be23-080027de035f/volumes/volumes.k8s.elastic.co~elastic-local/pvc-cc6199eb-eca0-11e8-be23-080027de035f"
// we want to return "pvc-cc6199eb-eca0-11e8-be23-080027de035f"
//
func ExtractPVCID(podVolumePath string) string {
	return path.Base(podVolumePath)
}

// BuildSourceDir builds the path to create the volume into,
// eg. /mnt/elastic-local-volumes/<pvc-name>
//
func BuildSourceDir(targetDir string) string {
	return path.Join(VolumesPath, ExtractPVCID(targetDir))
}

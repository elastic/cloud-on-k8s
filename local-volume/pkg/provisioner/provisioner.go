package provisioner

import (
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	"github.com/elastic/stack-operators/local-volume/pkg/k8s"
	"github.com/elastic/stack-operators/local-volume/pkg/provider"
	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Start the provisioner
func Start() error {
	provisioner := flexProvisioner{}

	k8sClient, err := k8s.NewClient()
	if err != nil {
		return err
	}
	k8sVersion, err := k8sClient.ClientSet.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	// run provisioner controller
	pc := controller.NewProvisionController(
		k8sClient.ClientSet,
		provider.Name,
		provisioner,
		k8sVersion.String(),
	)
	pc.Run(wait.NeverStop)

	return nil
}

// flexProvisioner is our implementation of k8s Provisioner interface
type flexProvisioner struct{}

// Provision creates a storage asset and returns a pv object representing it.
func (p flexProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	// retrieve storage size, if specified, else default to 0
	var storageInBytes int64
	requestedStorage, specified := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	if specified {
		storageInBytes = requestedStorage.Value()
	}

	pv := v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): requestedStorage,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexPersistentVolumeSource{
					Driver: provider.Name,
					Options: protocol.MountOptions{
						SizeBytes: storageInBytes,
					}.AsStrMap(),
				},
			},
		},
	}

	log.Infof("Provisioning persistent volume %s", provider.Name)

	return &pv, nil
}

// Delete moves the storage asset that was created by Provision represented by the given pv.
func (p flexProvisioner) Delete(volume *v1.PersistentVolume) error {
	// TODO
	return nil
}

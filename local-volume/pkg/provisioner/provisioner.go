package provisioner

import (
	drivermodel "github.com/elastic/localvolume/pkg/driver/model"
	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Start the provisioner
func Start() error {
	provisioner := flexProvisioner{}

	// create k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	k8sVersion, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	// run provisioner controller
	pc := controller.NewProvisionController(
		k8sClient,
		drivermodel.Name,
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

	// TODO: we could parse options.Parameters here to do whatever custom we want

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexPersistentVolumeSource{
					Driver: drivermodel.Name,
					// TODO: we can pass whatever we need to the driver here
					// Options: map[string]string{
					// },
				},
			},
		},
	}

	log.Infof("Provisioning persistent volume %s", drivermodel.Name)

	return pv, nil
}

// Delete moves the storage asset that was created by Provision represented by the given pv.
func (p flexProvisioner) Delete(volume *v1.PersistentVolume) error {
	// TODO
	return nil
}

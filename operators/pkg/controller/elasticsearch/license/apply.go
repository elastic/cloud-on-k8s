package license

import (
	"context"
	"errors"
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func applyLinkedLicense(
	c k8s.Client,
	esCluster types.NamespacedName,
	updater func(license v1alpha1.ClusterLicense) error,
) error {
	var license v1alpha1.ClusterLicense
	// the underlying assumption here is that either a user or a
	// license controller has created a cluster license in the
	// namespace of this cluster with the same name as the cluster
	err := c.Get(esCluster, &license)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// no license linked to this cluster. Expected for clusters running on trial
			return nil
		}
		return err
	}
	if license.IsEmpty() {
		return errors.New("empty license linked to this cluster")
	}
	return updater(license)
}

func secretRefResolver(c k8s.Client, ref corev1.SecretReference) func() (string, error) {
	return func() (string, error) {
		var secret corev1.Secret
		err := c.Get(types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &secret)
		if err != nil {
			return "", err
		}
		if len(secret.Data) != 1 {
			return "", errors.New("not exactly one secret element found but no key specified") // TODO support keys
		}
		for _, v := range secret.Data {
			return string(v), nil
		}
		return "", nil
	}
}

func updateLicense(
	c *esclient.Client,
	current *esclient.License,
	desired v1alpha1.ClusterLicense,
	sigResolver func() (string, error),
) error {
	if current != nil && current.UID == desired.Spec.UID {
		return nil // we are done already applied
	}
	sig, err := sigResolver()
	if err != nil {
		return err
	}
	request := esclient.LicenseUpdateRequest{
		Licenses: []esclient.License{
			{

				UID:                desired.Spec.UID,
				Type:               desired.Spec.Type,
				IssueDateInMillis:  desired.Spec.IssueDateInMillis,
				ExpiryDateInMillis: desired.Spec.ExpiryDateInMillis,
				MaxNodes:           desired.Spec.MaxNodes,
				IssuedTo:           desired.Spec.IssuedTo,
				Issuer:             desired.Spec.Issuer,
				StartDateInMillis:  desired.Spec.StartDateInMillis,
				Signature:          sig,
			},
		},
	}
	response, err := c.UpdateLicense(context.TODO(), request)
	if err != nil {
		return err
	}
	if !response.IsSuccess() {
		return fmt.Errorf("failed to apply license: %s", response.LicenseStatus)
	}
	return nil
}

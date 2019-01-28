package license

import (
	"context"
	"errors"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	c client.Client,
	clusterClient *esclient.Client,
	current *esclient.License,
) error {

	var labelSelector labels.Selector // TODO how to list linked licenses (by owner)?
	var licenseList v1alpha1.ClusterLicenseList
	err := c.List(context.TODO(), &client.ListOptions{LabelSelector: labelSelector}, &licenseList)
	if err != nil {
		return err
	}
	switch len(licenseList.Items) {
	case 1:
		license := licenseList.Items[0]
		sigResolver := secretRefResolver(c, license.Spec.SignatureRef)
		return updateLicense(clusterClient, current, license, sigResolver)
	case 0:
		return errors.New("no licenseList linked to this cluster")
	default:
		// TODO be smart here and select the most appropriate one?
		return errors.New("more than one licenseList linked to this cluster")
	}
}

func secretRefResolver(c client.Client, ref corev1.SecretReference) func() (string, error) {
	return func() (string, error) {
		var secret corev1.Secret
		err := c.Get(context.TODO(), types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &secret)
		if err != nil {
			return "", err
		}
		if len(secret.Data) != 1 {
			return "", errors.New("not exactly one secret element found but no key specified") // TODO support keys
		}
		for _, v := range secret.Data {
			return string(v), nil
		}
		return "", errors.New("empty secret -- no data found")
	}
}

func updateLicense(
	c *esclient.Client,
	current *esclient.License,
	desired v1alpha1.ClusterLicense,
	sigResolver func() (string, error)) error {
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
	if response.IsSuccess() {
		return nil
	}
	return fmt.Errorf("failed to apply license: %s", response.LicenseStatus)
}

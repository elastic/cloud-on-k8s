/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/pkg/errors"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
)

func CommercialFeaturesEnabled(client k8s.Client) bool {
	var licenses v1alpha1.EnterpriseLicenseList
	err := client.List(&client2.ListOptions{}, &licenses)
	if err != nil {
		log.Error(err, "while reading licenses")
		return false
	}

	for _, l := range licenses.Items {
		sigRef := l.Spec.SignatureRef
		var signtureSec corev1.Secret

		err := client.Get(types.NamespacedName{
			Namespace: "", // TODO ns!
			Name:      sigRef.Name,
		}, &signtureSec)
		if err != nil {
			log.Error(err, "while loading signature secret")
			return false
		}
		pk := publicKeyBytes
		if l.Spec.Type == v1alpha1.LicenseTypeEnterpriseTrial {
			pk = signtureSec.Data["pubkey"] // TODO const
		}
		if err := Valid(l, pk, time.Now(), signtureSec.Data[sigRef.Key]); err != nil {
			continue
		}
		return true
	}
	return false

}

func Valid(lic v1alpha1.EnterpriseLicense, pubKey []byte, now time.Time, sig []byte) error {
	if !lic.IsValid(now) {
		// do the cheap check first
		return errors.New("license expired")
	}
	verifier, err := NewVerifier(pubKey)
	if err != nil {
		return errors.Wrap(err, "while creating license verifier")
	}
	err = verifier.Valid(lic, sig)
	if err != nil {
		return errors.Wrap(err, "invalid license")
	}
	return nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func MaybeRetrieveAdditionalCAs(ctx context.Context, client k8s.Client, elasticsearch v1.Elasticsearch) ([]byte, error) {
	additionalTrust := elasticsearch.Spec.Transport.TLS.CertificateAuthorities
	if !additionalTrust.IsDefined() {
		return nil, nil
	}

	var configMap corev1.ConfigMap
	nsn := types.NamespacedName{Namespace: elasticsearch.Namespace, Name: additionalTrust.ConfigMapName}
	if err := client.Get(ctx, nsn, &configMap); err != nil {
		return nil, fmt.Errorf("could not retrieve config map %s specified in spec.transport.tls.certificateAuthorities: %w", nsn, err)
	}
	bytes, exists := configMap.Data[certificates.CAFileName]
	if !exists {
		return nil, fmt.Errorf("config map %s specified in spec.transport.tls.certificateAuthorities must contain ca.crt file", nsn)
	}

	return []byte(bytes), nil
}

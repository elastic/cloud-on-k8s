// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// AdditionalCAWatchKey for config maps holding additional CAs for the transport layer.
func AdditionalCAWatchKey(name types.NamespacedName) string {
	return fmt.Sprintf("%s-transport-ca-trust", name)
}

// ReconcileAdditionalCAs retrieves additional trust from an optional config map if configured and reconciles a watch for the config map.
func ReconcileAdditionalCAs(ctx context.Context, client k8s.Client, elasticsearch v1.Elasticsearch, watches watches.DynamicWatches) ([]byte, error) {
	owner := k8s.ExtractNamespacedName(&elasticsearch)
	return certificates.ReconcileAdditionalCAs(ctx, client, watches, certificates.AdditionalCAsOptions{
		Owner:     owner,
		Ref:       elasticsearch.Spec.Transport.TLS.CertificateAuthorities,
		WatchKey:  AdditionalCAWatchKey(owner),
		FieldPath: "spec.transport.tls.certificateAuthorities",
	})
}

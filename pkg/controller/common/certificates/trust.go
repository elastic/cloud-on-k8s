// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// AdditionalCAsOptions identifies the additional CAs to load and the configuration they come from.
type AdditionalCAsOptions struct {
	// Owner is the resource configuring the additional CAs. The ConfigMap is looked up in its namespace and the owner is reconciled when it changes.
	Owner types.NamespacedName
	// Ref references the ConfigMap holding the CAs. An undefined ref releases the watch.
	Ref commonv1.ConfigMapRef
	// WatchKey is the key of the ConfigMap watch.
	WatchKey string
	// FieldPath identifies the configuration field in error messages.
	FieldPath string
}

// ReconcileAdditionalCAs loads and validates certificates from the referenced ConfigMap and keeps its watch in sync.
// The watch is registered before loading the ConfigMap so creating a missing ConfigMap triggers reconciliation.
// Only certificates are returned, encoded as PEM with a trailing newline; other PEM blocks are discarded.
func ReconcileAdditionalCAs(
	ctx context.Context,
	c k8s.Client,
	dynamicWatches watches.DynamicWatches,
	opts AdditionalCAsOptions,
) ([]byte, error) {
	owner, ref, watchKey, fieldPath := opts.Owner, opts.Ref, opts.WatchKey, opts.FieldPath
	if !ref.IsDefined() {
		dynamicWatches.ConfigMaps.RemoveHandlerForKey(watchKey)
		return nil, nil
	}
	nsn := types.NamespacedName{Namespace: owner.Namespace, Name: ref.ConfigMapName}
	if err := dynamicWatches.ConfigMaps.AddHandler(watches.NamedWatch[*corev1.ConfigMap]{
		Name:    watchKey,
		Watched: []types.NamespacedName{nsn},
		Watcher: owner,
	}); err != nil {
		return nil, err
	}
	var configMap corev1.ConfigMap
	if err := c.Get(ctx, nsn, &configMap); err != nil {
		return nil, fmt.Errorf("could not retrieve config map %s specified in %s: %w", nsn, fieldPath, err)
	}
	pem, exists := configMap.Data[CAFileName]
	if !exists {
		return nil, fmt.Errorf("config map %s specified in %s must contain %s file", nsn, fieldPath, CAFileName)
	}
	return parseCAFile(fmt.Sprintf("config map %s", nsn), fieldPath, []byte(pem), false)
}

// parseCAFile parses the certificates of a ca.crt file and returns them PEM-encoded, dropping anything else.
// If caOnly is set, certificates which are not CAs are rejected.
func parseCAFile(source, fieldPath string, pem []byte, caOnly bool) ([]byte, error) {
	certs, err := ParsePEMCerts(pem)
	if err != nil {
		return nil, fmt.Errorf("%s specified in %s contains invalid certificates: %w", source, fieldPath, err)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("%s specified in %s contains invalid certificates: no certificate found in %s", source, fieldPath, CAFileName)
	}
	// Only hand out certificates: anything else, for example a private key, is dropped.
	raws := make([][]byte, len(certs))
	for i, cert := range certs {
		if caOnly && !cert.IsCA {
			return nil, fmt.Errorf("%s specified in %s contains invalid certificates: %q in %s is not a CA certificate",
				source, fieldPath, cert.Subject.String(), CAFileName)
		}
		raws[i] = cert.Raw
	}
	return EncodePEMCert(raws...), nil
}

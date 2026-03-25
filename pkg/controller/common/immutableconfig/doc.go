// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package immutableconfig provides utilities for managing immutable, content-addressed
// Kubernetes configuration resources (Secrets and ConfigMaps).
//
// The key idea is to use content-addressed naming: each configuration resource gets a
// name that includes a hash of its content (e.g., "my-config-a1b2c3d4"). This ensures
// that during rolling updates, old and new pod templates reference different configuration
// resources, preventing race conditions where replacement pods might boot with the wrong
// configuration.
//
// # Usage
//
// Controllers using this package should:
//  1. Define a Classifier that maps config file names to Immutable or Dynamic
//  2. Use BuildImmutableSecret/BuildImmutableConfigMap to create content-addressed resources
//  3. Build Revisions with NewRevisions(...).With...().Build(), then use ForSecretVolumes/
//     ForConfigMapVolumes to reconcile, patch volumes, and GC
//
// # Example
//
//	// Classifier for config files (determines what goes in immutable vs dynamic secrets)
//	fileClassifier := immutableconfig.MapClassifier{
//	    "config.yml": immutableconfig.Immutable,
//	    "dynamic.yml": immutableconfig.Dynamic,
//	}
//
//	// Classifier for volumes (determines which volumes reference immutable resources)
//	volumeClassifier := immutableconfig.MapClassifier{
//	    "config-volume": immutableconfig.Immutable,
//	}
//
//	immutableData, dynamicData, err := immutableconfig.SplitByClassification(allData, fileClassifier)
//	if err != nil {
//	    return err
//	}
//
//	secret := immutableconfig.BuildImmutableSecret("my-config", namespace, immutableData, labels)
//
//	// Labels to select immutable config resources for GC
//	resourceLabels := map[string]string{
//	    "app":                              "elasticsearch",
//	    immutableconfig.ConfigTypeLabelName: immutableconfig.ConfigTypeImmutable,
//	}
//
//	revisions, err := immutableconfig.NewRevisions(client, owner, namespace).
//	    WithConfigResourceSelector(resourceLabels).
//	    WithPodTemplateSource(immutableconfig.NewReplicaSetExtractor(rsLabels)).
//	    Build()
//
//	if err != nil {
//	    return err
//	}
//	secretRev := revisions.ForSecretVolumes(volumeClassifier)
//
//	name, err := secretRev.Reconcile(ctx, &secret)
//	if err != nil {
//	    return err
//	}
//	secretRev.PatchVolumes(podSpec.Volumes, name)
//
//	// After reconciling all resources:
//	if err := secretRev.GC(ctx); err != nil {
//	    return err
//	}
package immutableconfig

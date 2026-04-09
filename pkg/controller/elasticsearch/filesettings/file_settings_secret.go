// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	commonlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// Secret provides a consistent API to load, modify, and save
// the Elasticsearch file-based settings Secret.
//
// Usage:
//
//	fs, err := Load(ctx, c, esNsn, isStateless, meta)
//	fs.ApplyPolicy(policy, secretSources)  // SCP controller
//	// or: fs.SetClusterSecrets(secrets)   // ES controller
//	// or: fs.Reset()                      // clear to empty
//	fs.Save(ctx, c, owner)
type Secret struct {
	es          types.NamespacedName
	isStateless bool
	current     *corev1.Secret
	settings    Settings
	meta        metadata.Metadata
	version     int64

	secureSettingsSources []commonv1.NamespacedSecretSource

	// settingsCorrupted is true when the current Secret exists but its settings.json
	// could not be parsed. Patch operations (SetClusterSecrets) fail-fast in this case
	// to avoid overwriting fields managed by other controllers.
	settingsCorrupted bool
}

// Load fetches the current file settings Secret from the API server and returns
// a Secret initialized with the current settings state. If the Secret
// does not exist, the settings start empty.
func Load(ctx context.Context, c k8s.Client, es types.NamespacedName, isStateless bool, meta metadata.Metadata) (*Secret, error) {
	f := &Secret{
		es:          es,
		isStateless: isStateless,
		meta:        meta,
		version:     time.Now().UnixNano(),
	}

	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)}, &secret)
	if apierrors.IsNotFound(err) {
		f.settings = NewEmptySettings(f.version, isStateless)
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	f.current = &secret

	// Parse current settings from the Secret.
	if data, ok := secret.Data[SettingsSecretKey]; ok {
		if err := json.Unmarshal(data, &f.settings); err != nil {
			ulog.FromContext(ctx).Error(err, "Failed to unmarshal current file settings, starting from empty")
			f.settings = NewEmptySettings(f.version, isStateless)
			f.settingsCorrupted = true
		} else {
			f.settings.IsStateless = isStateless
		}
	} else {
		f.settings = NewEmptySettings(f.version, isStateless)
	}

	return f, nil
}

// Exists returns true if the Secret already exists on the API server.
func (f *Secret) Exists() bool {
	return f.current != nil
}

// Version returns the settings version that will be written on Save.
// The version is adjusted during Save if the settings hash is unchanged.
// Call after Save to get the final version.
func (f *Secret) Version() int64 {
	return f.version
}

// Reset clears the settings state to empty. Use when the Secret should be
// reset (e.g. when all StackConfigPolicy owners are removed).
func (f *Secret) Reset() *Secret {
	f.settings = NewEmptySettings(f.version, f.isStateless)
	f.secureSettingsSources = nil
	f.settingsCorrupted = false
	return f
}

// ApplyPolicy replaces the settings state from a StackConfigPolicy.
// For stateless clusters, cluster_secrets are preserved since they are
// managed separately by the ES controller.
// Unlike SetClusterSecrets, corruption is not checked here because ApplyPolicy
// replaces the full state — corrupted prior settings are safely discarded.
func (f *Secret) ApplyPolicy(policy policyv1alpha1.ElasticsearchConfigPolicySpec, secretSources []commonv1.NamespacedSecretSource) error {
	savedClusterSecrets := f.settings.State.ClusterSecrets
	if err := f.settings.updateState(f.es, policy); err != nil {
		return err
	}
	if f.isStateless {
		f.settings.State.ClusterSecrets = savedClusterSecrets
	}
	f.secureSettingsSources = secretSources
	return nil
}

// SetClusterSecrets patches only the cluster_secrets field in the settings,
// preserving all other fields (cluster_settings, ILM, etc.) from the current Secret.
// Returns an error if the current settings are malformed to avoid overwriting
// fields managed by other controllers.
func (f *Secret) SetClusterSecrets(secrets *commonv1.Config) error {
	if f.settingsCorrupted {
		return fmt.Errorf("cannot patch cluster_secrets: current file settings in %s are malformed", esv1.FileSettingsSecretName(f.es.Name))
	}
	f.settings.State.ClusterSecrets = secrets
	return nil
}

// SaveOpt configures Save behavior.
type SaveOpt func(*saveOpts)

type saveOpts struct {
	additiveOnly bool
	mutators     []func(*corev1.Secret) error
}

// WithAdditiveMetadata makes Save merge labels/annotations additively without
// removing managed ones missing from expected. Use when the caller doesn't own
// the full set of managed labels/annotations (e.g., ES controller patching a
// Secret also managed by the SCP controller).
func WithAdditiveMetadata() SaveOpt {
	return func(o *saveOpts) { o.additiveOnly = true }
}

// WithMutator adds a function that mutates the Secret after it has been fully
// assembled. On create, it runs on the expected Secret. On update, it runs on
// the reconciled Secret (current + expected merged) before the update check.
// Use for caller-specific metadata like soft owner references.
func WithMutator(fn func(*corev1.Secret) error) SaveOpt {
	return func(o *saveOpts) { o.mutators = append(o.mutators, fn) }
}

// Save persists the file settings Secret to the API server.
// Creates the Secret if it doesn't exist, updates it only if the content changed.
// The settings version is kept unchanged when the settings hash has not changed.
func (f *Secret) Save(ctx context.Context, c k8s.Client, owner client.Object, opts ...SaveOpt) error {
	var o saveOpts
	for _, opt := range opts {
		opt(&o)
	}

	// Keep old version if settings hash unchanged.
	newHash := f.settings.hash()
	if f.current != nil && f.current.Annotations[commonannotation.SettingsHashAnnotationName] == newHash {
		oldVersion, err := extractVersion(*f.current)
		if err != nil {
			return err
		}
		f.version = oldVersion
	}
	f.settings.Metadata.Version = strconv.FormatInt(f.version, 10)

	expected, err := f.buildSecret(newHash)
	if err != nil {
		return err
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, &expected, scheme.Scheme); err != nil {
			return err
		}
	}

	// Create path: apply mutators to expected, then create.
	if f.current == nil {
		for _, m := range o.mutators {
			if err := m(&expected); err != nil {
				return err
			}
		}
		return c.Create(ctx, &expected)
	}

	// Update path: merge expected into a copy of current, apply mutators, then update.
	reconciled := f.current.DeepCopy()
	applyExpectedSecret(reconciled, expected, fileSettingsManagedAnnotations, o.additiveOnly)
	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, reconciled, scheme.Scheme); err != nil {
			return err
		}
	}
	for _, m := range o.mutators {
		if err := m(reconciled); err != nil {
			return err
		}
	}
	if !isSecretUpdateNeeded(ctx, *f.current, *reconciled) {
		return nil
	}
	return c.Update(ctx, reconciled)
}

// isSecretUpdateNeeded compares the current Secret (as read from the API server) against
// the reconciled Secret (current + all expected mutations applied) and returns true if
// any field differs. This includes labels, annotations, data, and owner references.
// This uses reflect.DeepEqual for exact comparison because Save performs its own merge
// (applyExpectedSecret) before calling this. The reconcileSecret path in reconciler.go
// uses subset-based comparison instead because ReconcileResource handles merging separately.
func isSecretUpdateNeeded(ctx context.Context, current, reconciled corev1.Secret) bool {
	labelsChanged := !reflect.DeepEqual(current.Labels, reconciled.Labels)
	annotationsChanged := !reflect.DeepEqual(current.Annotations, reconciled.Annotations)
	dataChanged := !reflect.DeepEqual(current.Data, reconciled.Data)
	ownerRefsChanged := !reflect.DeepEqual(current.OwnerReferences, reconciled.OwnerReferences)

	if labelsChanged || annotationsChanged || dataChanged || ownerRefsChanged {
		ulog.FromContext(ctx).V(1).Info("Secret needs update",
			"secret_namespace", current.Namespace, "secret_name", current.Name,
			"labels_changed", labelsChanged, "annotations_changed", annotationsChanged,
			"data_changed", dataChanged, "owner_refs_changed", ownerRefsChanged,
		)
		return true
	}
	return false
}

// buildSecret constructs the expected corev1.Secret from the current settings state.
func (f *Secret) buildSecret(hash string) (corev1.Secret, error) {
	secretMeta := f.meta.Merge(metadata.Metadata{
		Annotations: map[string]string{
			commonannotation.SettingsHashAnnotationName: hash,
		},
	})

	settingsBytes, err := json.Marshal(f.settings)
	if err != nil {
		return corev1.Secret{}, err
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   f.es.Namespace,
			Name:        esv1.FileSettingsSecretName(f.es.Name),
			Labels:      secretMeta.Labels,
			Annotations: secretMeta.Annotations,
		},
		Data: map[string][]byte{
			SettingsSecretKey: settingsBytes,
		},
	}

	if err := setSecureSettings(&secret, f.secureSettingsSources); err != nil {
		return corev1.Secret{}, err
	}

	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	secret.Labels[commonlabel.StackConfigPolicyOnDeleteLabelName] = commonlabel.OrphanSecretResetOnPolicyDelete

	return secret, nil
}

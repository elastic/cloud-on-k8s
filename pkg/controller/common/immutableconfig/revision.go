// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"maps"
	"slices"
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// resourceOps encapsulates type-specific operations for Secrets vs ConfigMaps.
type resourceOps struct {
	// extractVolRef returns the resource name referenced by a volume, or "" if the
	// volume type doesn't match (e.g. a ConfigMap volume when looking for Secrets).
	// It does not filter by naming convention; GC scopes deletion candidates by label,
	// so non-immutable names in the protected set are harmless no-ops.
	extractVolRef func(vol corev1.Volume) string
	patchVolRef   func(vol *corev1.Volume, name string)
	newList       func() client.ObjectList
	newObj        func() client.Object
	kind          string
}

func (ops resourceOps) listNames(ctx context.Context, c client.Client, namespace string, labels client.MatchingLabels) ([]string, error) {
	list := ops.newList()
	if err := c.List(ctx, list, client.InNamespace(namespace), labels); err != nil {
		return nil, err
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		a, err := meta.Accessor(item)
		if err != nil {
			return nil, err
		}
		names = append(names, a.GetName())
	}
	return names, nil
}

func (ops resourceOps) deleteByName(ctx context.Context, c client.Client, namespace, name string) error {
	obj := ops.newObj()
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return c.Delete(ctx, obj)
}

// secretOps adapts resourceOps for Kubernetes Secrets, reading and writing
// volume references via vol.Secret.SecretName.
var secretOps = resourceOps{
	extractVolRef: func(vol corev1.Volume) string {
		if vol.Secret != nil {
			return vol.Secret.SecretName
		}
		return ""
	},
	patchVolRef: func(vol *corev1.Volume, name string) {
		if vol.Secret != nil {
			vol.Secret.SecretName = name
		}
	},
	newList: func() client.ObjectList { return &corev1.SecretList{} },
	newObj:  func() client.Object { return &corev1.Secret{} },
	kind:    "Secret",
}

// configMapOps adapts resourceOps for Kubernetes ConfigMaps, reading and writing
// volume references via vol.ConfigMap.Name.
var configMapOps = resourceOps{
	extractVolRef: func(vol corev1.Volume) string {
		if vol.ConfigMap != nil {
			return vol.ConfigMap.Name
		}
		return ""
	},
	patchVolRef: func(vol *corev1.Volume, name string) {
		if vol.ConfigMap != nil {
			vol.ConfigMap.Name = name
		}
	},
	newList: func() client.ObjectList { return &corev1.ConfigMapList{} },
	newObj:  func() client.Object { return &corev1.ConfigMap{} },
	kind:    "ConfigMap",
}

// Revision manages the lifecycle of immutable content-addressed resources (Secrets or ConfigMaps):
// create-only reconciliation with owner references, volume patching, and
// garbage collection that protects resources referenced by existing pod templates.
type Revision struct {
	client           client.Client
	owner            client.Object
	namespace        string
	resourceSelector client.MatchingLabels
	// podTemplateExtractor extracts pod templates from resources for GC protection.
	podTemplateExtractor PodTemplateExtractor
	// volumeNames lists the pod volume names that reference this immutable resource.
	// PatchVolumes updates all matching volumes; GC protects resources referenced
	// by any of these volumes in existing pod templates.
	volumeNames []string
	// reconciled tracks immutable resource names created or observed during the current
	// reconciliation cycle. These names are always protected from deletion by GC.
	reconciled sets.Set[string]
	ops        resourceOps
}

// Revisions is a builder that captures the common parameters shared across
// all Revisions for a given owner, and creates per-volume Secret or ConfigMap Revisions.
type Revisions struct {
	client               client.Client
	owner                client.Object
	namespace            string
	resourceSelector     client.MatchingLabels
	podTemplateExtractor PodTemplateExtractor
}

// RevisionsBuilder captures construction parameters for Revisions and validates
// required selectors before producing a usable Revisions value.
type RevisionsBuilder struct {
	client               client.Client
	owner                client.Object
	namespace            string
	resourceSelector     client.MatchingLabels
	podTemplateExtractor PodTemplateExtractor
}

func copyMatchingLabels(labels client.MatchingLabels) client.MatchingLabels {
	if labels == nil {
		return nil
	}
	copied := make(client.MatchingLabels, len(labels))
	maps.Copy(copied, labels)
	return copied
}

// NewRevisions creates a builder for Revisions that share the same client, owner, and namespace.
func NewRevisions(c client.Client, owner client.Object, namespace string) RevisionsBuilder {
	return RevisionsBuilder{
		client:    c,
		owner:     owner,
		namespace: namespace,
	}
}

// WithConfigResourceSelector sets the labels that select immutable configuration resources
// (Secrets and ConfigMaps) managed by this Revisions instance. These labels are used during
// garbage collection to identify which resources are candidates for deletion.
// If ConfigTypeLabelName is missing, Build() adds ConfigTypeImmutable automatically.
// If ConfigTypeLabelName is present with a different value, Build() returns an error.
func (b RevisionsBuilder) WithConfigResourceSelector(labels client.MatchingLabels) RevisionsBuilder {
	b.resourceSelector = copyMatchingLabels(labels)
	return b
}

// WithPodTemplateSource sets the extractor used to find pod templates whose volume
// references should be protected from garbage collection. The extractor encapsulates
// both the resource type and the label selector.
//
// Example for Deployments:
//
//	revisions, err := immutableconfig.NewRevisions(client, owner, namespace).
//	    WithConfigResourceSelector(resourceLabels).
//	    WithPodTemplateSource(immutableconfig.NewReplicaSetExtractor(rsLabels)).
//	    Build()
func (b RevisionsBuilder) WithPodTemplateSource(extractor PodTemplateExtractor) RevisionsBuilder {
	b.podTemplateExtractor = extractor
	return b
}

// Build validates builder configuration and returns a Revisions value.
func (b RevisionsBuilder) Build() (Revisions, error) {
	if b.client == nil {
		return Revisions{}, errors.New("client is required")
	}
	if b.namespace == "" {
		return Revisions{}, errors.New("namespace is required")
	}
	if len(b.resourceSelector) == 0 {
		return Revisions{}, errors.New("config resource selector is required (use WithConfigResourceSelector)")
	}
	selector := copyMatchingLabels(b.resourceSelector)
	if value, exists := selector[ConfigTypeLabelName]; !exists {
		selector[ConfigTypeLabelName] = ConfigTypeImmutable
	} else if value != ConfigTypeImmutable {
		return Revisions{}, fmt.Errorf(
			"config resource selector must include %s=%s",
			ConfigTypeLabelName,
			ConfigTypeImmutable,
		)
	}
	if b.podTemplateExtractor == nil {
		return Revisions{}, errors.New("pod template source is required (use WithPodTemplateSource)")
	}
	return Revisions{
		client:               b.client,
		owner:                b.owner,
		namespace:            b.namespace,
		resourceSelector:     selector,
		podTemplateExtractor: b.podTemplateExtractor,
	}, nil
}

// ForSecretVolumes creates a Revision for an immutable Secret using a classifier to determine
// which volumes are immutable. The classifier is the single source of truth: volumes classified
// as Immutable will be patched by PatchVolumes and protected during GC.
// The classifier should contain at least one Immutable entry; otherwise the Revision
// will patch nothing and protect nothing during GC.
func (b Revisions) ForSecretVolumes(classifier MapClassifier) *Revision {
	return &Revision{
		client:               b.client,
		owner:                b.owner,
		namespace:            b.namespace,
		resourceSelector:     b.resourceSelector,
		podTemplateExtractor: b.podTemplateExtractor,
		volumeNames:          classifier.NamesWithClassification(Immutable),
		reconciled:           sets.New[string](),
		ops:                  secretOps,
	}
}

// ForConfigMapVolumes creates a Revision for an immutable ConfigMap using a classifier to determine
// which volumes are immutable. The classifier is the single source of truth: volumes classified
// as Immutable will be patched by PatchVolumes and protected during GC.
// The classifier should contain at least one Immutable entry; otherwise the Revision
// will patch nothing and protect nothing during GC.
func (b Revisions) ForConfigMapVolumes(classifier MapClassifier) *Revision {
	return &Revision{
		client:               b.client,
		owner:                b.owner,
		namespace:            b.namespace,
		resourceSelector:     b.resourceSelector,
		podTemplateExtractor: b.podTemplateExtractor,
		volumeNames:          classifier.NamesWithClassification(Immutable),
		reconciled:           sets.New[string](),
		ops:                  configMapOps,
	}
}

// Reconcile creates the immutable resource if it does not already exist, sets the owner
// reference, and tracks the name for GC protection. Returns the content-addressed name.
// The object's namespace must match the Revision's namespace to ensure GC can find it.
func (r *Revision) Reconcile(ctx context.Context, obj client.Object) (string, error) {
	if obj.GetNamespace() != r.namespace {
		return "", fmt.Errorf("object namespace %q does not match Revision namespace %q", obj.GetNamespace(), r.namespace)
	}
	if r.owner != nil {
		if err := controllerutil.SetControllerReference(r.owner, obj, scheme.Scheme); err != nil {
			return "", err
		}
	}

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
	existing := obj.DeepCopyObject().(client.Object)
	err := r.client.Get(ctx, key, existing)
	if err == nil {
		r.reconciled.Insert(obj.GetName())
		return obj.GetName(), nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}
	if err := r.client.Create(ctx, obj); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}
	r.reconciled.Insert(obj.GetName())
	return obj.GetName(), nil
}

// PatchVolumes updates volume references in-place to point to the given immutable resource name.
// Only volumes matching any of the Revision's volume names are patched.
func (r *Revision) PatchVolumes(volumes []corev1.Volume, name string) {
	for i := range volumes {
		if slices.Contains(r.volumeNames, volumes[i].Name) {
			r.ops.patchVolRef(&volumes[i], name)
		}
	}
}

// GC deletes immutable resources that are no longer referenced by any pod template
// and were not reconciled in the current cycle.
func (r *Revision) GC(ctx context.Context) error {
	log := crlog.FromContext(ctx)

	// Collect names that must not be deleted: anything reconciled this cycle,
	// plus anything still referenced by an existing pod template's volumes.
	protectedNames := r.reconciled.Clone()

	templates, err := r.podTemplateExtractor.ListPodTemplates(ctx, r.client, r.namespace)
	if err != nil {
		return err
	}
	for _, template := range templates {
		r.collectReferencedNames(template.Spec.Volumes, protectedNames)
	}

	names, err := r.ops.listNames(ctx, r.client, r.namespace, r.resourceSelector)
	if err != nil {
		return err
	}
	for _, name := range names {
		if protectedNames.Has(name) {
			continue
		}
		log.Info("Deleting unreferenced immutable config "+r.ops.kind, "name", name)
		if err := r.ops.deleteByName(ctx, r.client, r.namespace, name); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			return err
		}
	}
	return nil
}

// collectReferencedNames extracts resource names from volumes matching volumeNames and adds them to the set.
// extractVolRef returns the volume reference unconditionally (including non-immutable names).
// This is safe because listNames only returns resources matching resourceSelector
// (which Build() ensures includes config-type=immutable),
// so a dynamic name in the set simply won't match any deletion candidate.
func (r *Revision) collectReferencedNames(volumes []corev1.Volume, names sets.Set[string]) {
	for _, vol := range volumes {
		if !slices.Contains(r.volumeNames, vol.Name) {
			continue
		}
		if name := r.ops.extractVolRef(vol); name != "" {
			names.Insert(name)
		}
	}
}

// GCAll runs garbage collection on all provided Revisions, collecting any errors.
func GCAll(ctx context.Context, revisions ...*Revision) error {
	errs := make([]error, len(revisions))
	for i, r := range revisions {
		errs[i] = r.GC(ctx)
	}
	return errors.Join(errs...)
}

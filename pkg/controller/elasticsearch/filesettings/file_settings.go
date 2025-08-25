// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

var (
	FileBasedSettingsMinPreVersion = version.MinFor(8, 6, 1)
	FileBasedSettingsMinVersion    = version.WithoutPre(FileBasedSettingsMinPreVersion)
)

// Settings represents the "File-based Settings" to write to the JSON file watched by Elasticsearch.
type Settings struct {
	Metadata SettingsMetadata `json:"metadata"`
	State    SettingsState    `json:"state"`
}

// SettingsMetadata represents the metadata of the "File-based Settings".
// Settings are versioned and any change in the Settings state must be followed by a version increase.
type SettingsMetadata struct {
	Version       string `json:"version"`
	Compatibility string `json:"compatibility"`
}

// SettingsState represents the state of the "File-based Settings".
// This is where the configuration of Elasticsearch objects resides.
type SettingsState struct {
	ClusterSettings        *commonv1.Config `json:"cluster_settings,omitempty"`
	SnapshotRepositories   *commonv1.Config `json:"snapshot_repositories,omitempty"`
	SLM                    *commonv1.Config `json:"slm,omitempty"`
	RoleMappings           *commonv1.Config `json:"role_mappings,omitempty"`
	IndexLifecyclePolicies *commonv1.Config `json:"ilm,omitempty"`
	IngestPipelines        *commonv1.Config `json:"ingest_pipelines,omitempty"`
	IndexTemplates         *IndexTemplates  `json:"index_templates,omitempty"`
}

type IndexTemplates struct {
	ComponentTemplates       *commonv1.Config `json:"component_templates,omitempty"`
	ComposableIndexTemplates *commonv1.Config `json:"composable_index_templates,omitempty"`
}

// hash returns the hash of the Settings, considering only the State without the Metadata.
func (s *Settings) hash() string {
	return hash.HashObject(s.State)
}

// NewEmptySettings returns empty new Settings.
func NewEmptySettings(version int64) Settings {
	return Settings{
		Metadata: SettingsMetadata{Version: fmt.Sprintf("%d", version), Compatibility: FileBasedSettingsMinVersion.String()},
		State:    newEmptySettingsState(),
	}
}

// newEmptySettingsState returns an empty new Settings state.
func newEmptySettingsState() SettingsState {
	return SettingsState{
		ClusterSettings:        &commonv1.Config{Data: map[string]interface{}{}},
		SnapshotRepositories:   &commonv1.Config{Data: map[string]interface{}{}},
		SLM:                    &commonv1.Config{Data: map[string]interface{}{}},
		RoleMappings:           &commonv1.Config{Data: map[string]interface{}{}},
		IndexLifecyclePolicies: &commonv1.Config{Data: map[string]interface{}{}},
		IngestPipelines:        &commonv1.Config{Data: map[string]interface{}{}},
		IndexTemplates: &IndexTemplates{
			ComponentTemplates:       &commonv1.Config{Data: map[string]interface{}{}},
			ComposableIndexTemplates: &commonv1.Config{Data: map[string]interface{}{}},
		},
	}
}

// updateStateFromPolicies merges settings from multiple StackConfigPolicies based on their weights.
// Lower weight policies override higher weight policies for conflicting settings.
func (s *Settings) updateStateFromPolicies(es types.NamespacedName, policies []policyv1alpha1.StackConfigPolicy) error {
	if len(policies) == 0 {
		return nil
	}

	sortedPolicies := make([]policyv1alpha1.StackConfigPolicy, len(policies))
	copy(sortedPolicies, policies)

	// Bubble sort by weight (descending order)
	for i := 0; i < len(sortedPolicies)-1; i++ {
		for j := 0; j < len(sortedPolicies)-i-1; j++ {
			if sortedPolicies[j].Spec.Weight < sortedPolicies[j+1].Spec.Weight {
				sortedPolicies[j], sortedPolicies[j+1] = sortedPolicies[j+1], sortedPolicies[j]
			}
		}
	}

	for _, policy := range sortedPolicies {
		if err := s.updateState(es, policy); err != nil {
			return err
		}
	}

	return nil
}

// updateState updates the Settings state from a StackConfigPolicy for a given Elasticsearch.
func (s *Settings) updateState(es types.NamespacedName, policy policyv1alpha1.StackConfigPolicy) error {
	p := policy.DeepCopy() // be sure to not mutate the original policy

	// Initialize state if not already done
	if s.State.ClusterSettings == nil {
		s.State = newEmptySettingsState()
	}

	// mutate Snapshot Repositories
	if p.Spec.Elasticsearch.SnapshotRepositories != nil {
		for name, untypedDefinition := range p.Spec.Elasticsearch.SnapshotRepositories.Data {
			definition, ok := untypedDefinition.(map[string]interface{})
			if !ok {
				return fmt.Errorf(`invalid type (%T) for definition of snapshot repository %q of Elasticsearch "%s/%s"`, untypedDefinition, name, es.Namespace, es.Name)
			}
			repoSettings, err := mutateSnapshotRepositorySettings(definition, es.Namespace, es.Name)
			if err != nil {
				return err
			}
			p.Spec.Elasticsearch.SnapshotRepositories.Data[name] = repoSettings
		}
		s.mergeConfig(s.State.SnapshotRepositories, p.Spec.Elasticsearch.SnapshotRepositories)
	}
	// merge other settings
	if p.Spec.Elasticsearch.ClusterSettings != nil {
		s.mergeConfig(s.State.ClusterSettings, p.Spec.Elasticsearch.ClusterSettings)
	}
	if p.Spec.Elasticsearch.SnapshotLifecyclePolicies != nil {
		s.mergeConfig(s.State.SLM, p.Spec.Elasticsearch.SnapshotLifecyclePolicies)
	}
	if p.Spec.Elasticsearch.SecurityRoleMappings != nil {
		s.mergeConfig(s.State.RoleMappings, p.Spec.Elasticsearch.SecurityRoleMappings)
	}
	if p.Spec.Elasticsearch.IndexLifecyclePolicies != nil {
		s.mergeConfig(s.State.IndexLifecyclePolicies, p.Spec.Elasticsearch.IndexLifecyclePolicies)
	}
	if p.Spec.Elasticsearch.IngestPipelines != nil {
		s.mergeConfig(s.State.IngestPipelines, p.Spec.Elasticsearch.IngestPipelines)
	}
	if p.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates != nil {
		s.mergeConfig(s.State.IndexTemplates.ComposableIndexTemplates, p.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates)
	}
	if p.Spec.Elasticsearch.IndexTemplates.ComponentTemplates != nil {
		s.mergeConfig(s.State.IndexTemplates.ComponentTemplates, p.Spec.Elasticsearch.IndexTemplates.ComponentTemplates)
	}
	return nil
}

// mergeConfig merges source config into target config, with source taking precedence
// For map-type values (like snapshot repositories), individual entries are merged rather than replaced
func (s *Settings) mergeConfig(target, source *commonv1.Config) {
	if source == nil || source.Data == nil {
		return
	}
	if target == nil || target.Data == nil {
		target = &commonv1.Config{Data: make(map[string]interface{})}
	}

	for key, value := range source.Data {
		if targetValue, exists := target.Data[key]; exists {
			if targetMap, targetIsMap := targetValue.(map[string]interface{}); targetIsMap {
				if sourceMap, sourceIsMap := value.(map[string]interface{}); sourceIsMap {
					for subKey, subValue := range sourceMap {
						targetMap[subKey] = subValue
					}
					continue
				}
			}
		}
		// For non-map values or if target doesn't exist, replace entirely
		target.Data[key] = value
	}
}

// mutateSnapshotRepositorySettings ensures that a snapshot repository can be used across multiple ES clusters.
// The namespace and the Elasticsearch cluster name are injected in the repository settings depending on the type of the repository:
// - "azure", "gcs", "s3": if not provided, the `base_path` property is set to `snapshots/<namespace>-<esName>`
// - "fs": `<namespace>-<esName>` is appended to the `location` property
// - "hdfs": `<namespace>-<esName>` is appended to the `path` property
// - "url": nothing is done, the repository is read-only
// - "source": nothing is done, the repository is an indirection to another repository
func mutateSnapshotRepositorySettings(snapshotRepository map[string]interface{}, esNs string, esName string) (map[string]interface{}, error) {
	untypedSettings := snapshotRepository["settings"]
	if untypedSettings == nil {
		untypedSettings = map[string]interface{}{}
	}

	uniqSuffix := fmt.Sprintf("%s-%s", esNs, esName)
	settings, ok := untypedSettings.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid type (%T) for snapshot repository settings", untypedSettings)
	}
	switch snapshotRepository["type"] {
	case "azure", "gcs", "s3":
		basePath, ok := settings["base_path"].(string)
		if !ok {
			// not provided, set a default `base_path` with a uniq suffix
			basePath = filepath.Join("snapshots", uniqSuffix)
		}
		settings["base_path"] = basePath
	case "fs":
		location, ok := settings["location"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid type (%T) for snapshot repository location", settings["location"])
		}
		// always append an uniq suffix
		settings["location"] = filepath.Join(location, uniqSuffix)
	case "hdfs":
		path, ok := settings["path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid type (%T) for snapshot repository path", settings["path"])
		}
		// always append an uniq suffix
		settings["path"] = filepath.Join(path, uniqSuffix)
	}
	snapshotRepository["settings"] = settings

	return snapshotRepository, nil
}

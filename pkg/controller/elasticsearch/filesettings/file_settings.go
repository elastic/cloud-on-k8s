// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	FileBasedSettingsMinPreVersion = version.MinFor(8, 6, 0)
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
	ClusterSettings      *commonv1.Config `json:"cluster_settings,omitempty"`
	SnapshotRepositories *commonv1.Config `json:"snapshot_repositories,omitempty"`
	SLM                  *commonv1.Config `json:"slm,omitempty"`
	// RoleMappings           *commonv1.Config `json:"role_mappings,omitempty"`
	// Autoscaling            *commonv1.Config `json:"autoscaling,omitempty"`
	// IndexLifecyclePolicies *commonv1.Config `json:"ilm,omitempty"`
	// IngestPipelines        *commonv1.Config `json:"ingest_pipelines,omitempty"`
	// IndexTemplates         *IndexTemplates  `json:"index_templates,omitempty"`
}

// type IndexTemplates struct {
//	ComponentTemplates       *commonv1.Config `json:"component_templates,omitempty"`
//	ComposableIndexTemplates *commonv1.Config `json:"composable_index_templates,omitempty"`
//}

// hash returns the hash of the Settings, considering only the State without the Metadata.
func (s *Settings) hash() string {
	return hash.HashObject(s.State)
}

// NewEmptySettings returns empty new Settings.
func NewEmptySettings(version int64) Settings {
	return Settings{
		Metadata: SettingsMetadata{Version: fmt.Sprintf("%d", version), Compatibility: FileBasedSettingsMinVersion.String()},
		State:    newSettingsState(),
	}
}

// newSettingsState returns an empty new Settings state.
func newSettingsState() SettingsState {
	return SettingsState{
		ClusterSettings:      &commonv1.Config{Data: map[string]interface{}{}},
		SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{}},
		SLM:                  &commonv1.Config{Data: map[string]interface{}{}},
	}
}

// updateState updates the Settings state from a StackConfigPolicy for a given Elasticsearch.
func (s *Settings) updateState(es types.NamespacedName, policy policyv1alpha1.StackConfigPolicy) error {
	p := policy.DeepCopy() // be sure to not mutate the original policy
	state := newSettingsState()
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
		state.SnapshotRepositories = p.Spec.Elasticsearch.SnapshotRepositories
	}
	// just copy other settings
	if p.Spec.Elasticsearch.ClusterSettings != nil {
		state.ClusterSettings = p.Spec.Elasticsearch.ClusterSettings
	}
	if p.Spec.Elasticsearch.SnapshotLifecyclePolicies != nil {
		state.SLM = p.Spec.Elasticsearch.SnapshotLifecyclePolicies
	}
	/*if p.Spec.Elasticsearch.SecurityRoleMappings != nil {
		state.RoleMappings = p.Spec.Elasticsearch.SecurityRoleMappings
	}
	if p.Spec.Elasticsearch.AutoscalingPolicies != nil {
		state.Autoscaling = p.Spec.Elasticsearch.AutoscalingPolicies
	}
	if p.Spec.Elasticsearch.IndexLifecyclePolicies != nil {
		state.IndexLifecyclePolicies = p.Spec.Elasticsearch.IndexLifecyclePolicies
	}
	if p.Spec.Elasticsearch.IngestPipelines != nil {
		state.IngestPipelines = p.Spec.Elasticsearch.IngestPipelines
	}
	if p.Spec.Elasticsearch.IndexTemplates != nil {
		if state.IndexTemplates == nil {
			state.IndexTemplates = &IndexTemplates{}
		}
		if p.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates != nil {
			state.IndexTemplates.ComposableIndexTemplates = p.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates
		}
		if p.Spec.Elasticsearch.IndexTemplates.ComponentTemplates != nil {
			state.IndexTemplates.ComponentTemplates = p.Spec.Elasticsearch.IndexTemplates.ComponentTemplates
		}
	}*/
	s.State = state
	return nil
}

// mutateSnapshotRepositorySettings ensures that a snapshot repository can be used across multiple ES clusters.
// The Elasticsearch cluster name is injected in the repository settings depending on the type of the repository:
// - "azure", "gcs", "s3": the `base_path` property is added is set to the cluster name
// - "fs": the cluster name is appended to the `location` property
// - "hdfs": the cluster name is appended to the `path` property
// - "url": nothing is done, the repository is read-only
// - "source": nothing is done, the repository is an indirection to another repository
func mutateSnapshotRepositorySettings(snapshotRepository map[string]interface{}, esNs string, esName string) (map[string]interface{}, error) {
	untypedSettings := snapshotRepository["settings"]
	if untypedSettings == nil {
		untypedSettings = map[string]interface{}{}
	}

	suffix := fmt.Sprintf("%s-%s", esNs, esName)
	settings, ok := untypedSettings.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid type (%T) for snapshot repository settings", untypedSettings)
	}
	switch snapshotRepository["type"] {
	case "azure", "gcs", "s3":
		settings["base_path"] = fmt.Sprintf("snapshots/%s", suffix)
	case "fs":
		location, ok := settings["location"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid type (%T) for snapshot repository location", settings["location"])
		}
		settings["location"] = filepath.Join(location, suffix)
	case "hdfs":
		path, ok := settings["path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid type (%T) for snapshot repository path", settings["path"])
		}
		settings["path"] = filepath.Join(path, suffix)
	}
	snapshotRepository["settings"] = settings

	return snapshotRepository, nil
}

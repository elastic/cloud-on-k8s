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
		ClusterSettings:        &commonv1.Config{Data: map[string]any{}},
		SnapshotRepositories:   &commonv1.Config{Data: map[string]any{}},
		SLM:                    &commonv1.Config{Data: map[string]any{}},
		RoleMappings:           &commonv1.Config{Data: map[string]any{}},
		IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{}},
		IngestPipelines:        &commonv1.Config{Data: map[string]any{}},
		IndexTemplates: &IndexTemplates{
			ComponentTemplates:       &commonv1.Config{Data: map[string]any{}},
			ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{}},
		},
	}
}

// updateState updates the Settings state from a StackConfigPolicy for a given Elasticsearch.
func (s *Settings) updateState(es types.NamespacedName, esConfigPolicy policyv1alpha1.ElasticsearchConfigPolicySpec) error {
	esConfigPolicy = *esConfigPolicy.DeepCopy() // be sure to not mutate the original es config policy
	state := newEmptySettingsState()
	// mutate Snapshot Repositories
	if esConfigPolicy.SnapshotRepositories != nil {
		for name, untypedDefinition := range esConfigPolicy.SnapshotRepositories.Data {
			definition, ok := untypedDefinition.(map[string]any)
			if !ok {
				return fmt.Errorf(`invalid type (%T) for definition of snapshot repository %q of Elasticsearch "%s/%s"`, untypedDefinition, name, es.Namespace, es.Name)
			}
			repoSettings, err := mutateSnapshotRepositorySettings(definition, es.Namespace, es.Name)
			if err != nil {
				return err
			}
			esConfigPolicy.SnapshotRepositories.Data[name] = repoSettings
		}
		state.SnapshotRepositories = esConfigPolicy.SnapshotRepositories
	}
	// just copy other settings
	if esConfigPolicy.ClusterSettings != nil {
		state.ClusterSettings = esConfigPolicy.ClusterSettings
	}
	if esConfigPolicy.SnapshotLifecyclePolicies != nil {
		state.SLM = esConfigPolicy.SnapshotLifecyclePolicies
	}
	if esConfigPolicy.SecurityRoleMappings != nil {
		state.RoleMappings = esConfigPolicy.SecurityRoleMappings
	}
	if esConfigPolicy.IndexLifecyclePolicies != nil {
		state.IndexLifecyclePolicies = esConfigPolicy.IndexLifecyclePolicies
	}
	if esConfigPolicy.IngestPipelines != nil {
		state.IngestPipelines = esConfigPolicy.IngestPipelines
	}
	if esConfigPolicy.IndexTemplates.ComposableIndexTemplates != nil {
		state.IndexTemplates.ComposableIndexTemplates = esConfigPolicy.IndexTemplates.ComposableIndexTemplates
	}
	if esConfigPolicy.IndexTemplates.ComponentTemplates != nil {
		state.IndexTemplates.ComponentTemplates = esConfigPolicy.IndexTemplates.ComponentTemplates
	}
	s.State = state
	return nil
}

// mutateSnapshotRepositorySettings ensures that a snapshot repository can be used across multiple ES clusters.
// The namespace and the Elasticsearch cluster name are injected in the repository settings depending on the type of the repository:
// - "azure", "gcs", "s3": if not provided, the `base_path` property is set to `snapshots/<namespace>-<esName>`
// - "fs": `<namespace>-<esName>` is appended to the `location` property
// - "hdfs": `<namespace>-<esName>` is appended to the `path` property
// - "url": nothing is done, the repository is read-only
// - "source": nothing is done, the repository is an indirection to another repository
func mutateSnapshotRepositorySettings(snapshotRepository map[string]any, esNs string, esName string) (map[string]any, error) {
	untypedSettings := snapshotRepository["settings"]
	if untypedSettings == nil {
		untypedSettings = map[string]any{}
	}

	uniqSuffix := fmt.Sprintf("%s-%s", esNs, esName)
	settings, ok := untypedSettings.(map[string]any)
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

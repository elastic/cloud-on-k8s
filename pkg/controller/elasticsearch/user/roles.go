// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"fmt"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"

	"gopkg.in/yaml.v3"
)

const (
	// RolesFile is the name of the roles file in the ES config dir.
	RolesFile = "roles.yml"

	// SuperUserBuiltinRole is the name of the built-in superuser role.
	SuperUserBuiltinRole = "superuser"
	// ProbeUserRole is the name of the role used by the internal probe user.
	ProbeUserRole = "elastic_internal_probe_user"
	// RemoteMonitoringCollectorBuiltinRole is the name of the built-in remote_monitoring_collector role.
	RemoteMonitoringCollectorBuiltinRole = "remote_monitoring_collector"

	// ApmUserRoleV6 is the name of the role used by 6.8.x APMServer instances to connect to Elasticsearch.
	ApmUserRoleV6 = "eck_apm_user_role_v6"
	// ApmUserRoleV7 is the name of the role used by APMServer instances to connect to Elasticsearch from version 7.1 to 7.4 included.
	ApmUserRoleV7 = "eck_apm_user_role_v7"
	// ApmUserRoleV75 is the name of the role used by APMServer instances to connect to Elasticsearch from version 7.5
	ApmUserRoleV75 = "eck_apm_user_role_v75"
	// ApmUserRoleV80 is the name of the role used by APMServer instances to connect to Elasticsearch from version 8.0
	ApmUserRoleV80 = "eck_apm_user_role_v80"

	// ApmAgentUserRole is the name of the role used by APMServer instances to connect to Kibana
	ApmAgentUserRole = "eck_apm_agent_user_role"

	// StackMonitoringMetricsUserRole is the name of the role used by Metricbeat and Filebeat to send metrics and log
	// data to the monitoring Elasticsearch cluster when Stack Monitoring is enabled
	StackMonitoringUserRole = "eck_stack_mon_user_role"

	FleetAdminUserRole = "eck_fleet_admin_user_role"

	LogstashUserRole = "eck_logstash_user_role_v80"

	// V70 indicates version 7.0
	V70 = "v70"

	// V73 indicates version 7.3
	V73 = "v73"

	// V75 indicates version 7.5
	V75 = "v75"

	// V77 indicates version 7.7
	V77 = "v77"
)

var (
	// PredefinedRoles to create for internal needs.
	PredefinedRoles = RolesFileContent{
		ProbeUserRole: esclient.Role{Cluster: []string{"monitor"}},
		ApmUserRoleV6: esclient.Role{
			Cluster: []string{"monitor", "manage_index_templates"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{"apm-*"},
					Privileges: []string{"write", "create_index"},
				},
			},
		},
		ApmUserRoleV7: esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_index_templates"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{"apm-*"},
					Privileges: []string{"manage", "write", "create_index"},
				},
			},
		},
		ApmUserRoleV75: esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_api_key"}, // manage_api_key has been introduced in 7.5
			Indices: []esclient.IndexRole{
				{
					Names:      []string{"apm-*"},
					Privileges: []string{"manage", "create_doc", "create_index"},
				},
			},
		},
		ApmUserRoleV80: esclient.Role{
			Cluster: []string{"cluster:monitor/main", "manage_index_templates"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{"traces-apm*", "metrics-apm*", "logs-apm*"},
					Privileges: []string{"auto_configure", "create_doc"},
				},
				{
					Names:      []string{"traces-apm.sampled-*"},
					Privileges: []string{"maintenance", "monitor", "read"},
				},
			},
		},
		ApmAgentUserRole: esclient.Role{
			Cluster: []string{},
			Indices: []esclient.IndexRole{},
			Applications: []esclient.ApplicationRole{
				{
					Application: "kibana-.kibana",
					Resources:   []string{"space:default"},
					Privileges:  []string{"feature_apm.read"},
				},
			},
		},
		// StackMonitoringUserRole is a dedicated role for Stack Monitoring with Metricbeat and Filebeat used for the
		// user sending monitoring data.
		// See: https://www.elastic.co/guide/en/beats/filebeat/7.14/privileges-to-publish-monitoring.html.
		StackMonitoringUserRole: esclient.Role{
			Cluster: []string{
				"monitor",
				"manage_index_templates",
				"manage_ingest_pipelines",
				"manage_ilm",
				"read_ilm",
				"cluster:admin/xpack/watcher/watch/put",
				"cluster:admin/xpack/watcher/watch/delete",
			},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{".monitoring-*"},
					Privileges: []string{"all"},
				},
				{
					Names:      []string{"metricbeat-*"},
					Privileges: []string{"manage", "read", "create_doc", "view_index_metadata", "create_index"},
				},
				{
					Names:      []string{"filebeat-*"},
					Privileges: []string{"manage", "read", "create_doc", "view_index_metadata", "create_index"},
				},
			},
		},
		FleetAdminUserRole: esclient.Role{
			Applications: []esclient.ApplicationRole{
				{
					Application: "kibana-.kibana",
					Resources:   []string{"*"},
					Privileges:  []string{"feature_fleet.all", "feature_fleetv2.all"},
				},
			},
		},
		LogstashUserRole: esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_ml", "read_ilm", "cluster:admin/ingest/pipeline/get"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{"logstash-*"},
					Privileges: []string{"manage", "read", "create_doc", "view_index_metadata", "create_index"},
				},
			},
		},
	}
)

func init() {
	for beat := range beatv1beta1.KnownTypes {
		PredefinedRoles[BeatEsRoleName(V77, beat)] = esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_ml", "read_ilm", "cluster:admin/ingest/pipeline/get"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read", "create_doc", "view_index_metadata", "create_index"},
				},
			},
		}

		PredefinedRoles[BeatEsRoleName(V75, beat)] = esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_ml", "read_ilm", "cluster:admin/ingest/pipeline/get"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read", "create_doc", "view_index_metadata", "create_index"},
				},
			},
		}

		PredefinedRoles[BeatEsRoleName(V73, beat)] = esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_ml", "read_ilm", "manage_pipeline"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read", "index", "view_index_metadata", "create_index"},
				},
			},
		}

		PredefinedRoles[BeatEsRoleName(V70, beat)] = esclient.Role{
			Cluster: []string{"manage_index_templates", "monitor", "manage_ilm", "manage_ml", "manage_pipeline"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read", "index", "create_index"},
				},
			},
		}

		PredefinedRoles[BeatKibanaRoleName(V77, beat)] = esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_ml"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read"},
				},
			},
		}

		PredefinedRoles[BeatKibanaRoleName(V73, beat)] = esclient.Role{
			Cluster: []string{"monitor", "manage_ilm", "manage_ml"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read"},
				},
			},
		}

		PredefinedRoles[BeatKibanaRoleName(V70, beat)] = esclient.Role{
			Cluster: []string{"manage_index_templates", "monitor", "manage_ilm", "manage_ml"},
			Indices: []esclient.IndexRole{
				{
					Names:      []string{fmt.Sprintf("%s-*", beat)},
					Privileges: []string{"manage", "read"},
				},
			},
		}
	}
}

func BeatEsRoleName(version, beatType string) string {
	return fmt.Sprintf("eck_beat_es_%s_role_%s", beatType, version)
}

func BeatKibanaRoleName(version, beatType string) string {
	return fmt.Sprintf("eck_beat_kibana_%s_role_%s", beatType, version)
}

// RolesFileContent is a map {role name -> yaml role spec}.
// We care about the role names here, but consider the roles spec as a yaml blob we don't need to access.
type RolesFileContent map[string]interface{}

// parseRolesFileContent returns a RolesFileContent from the given data.
// Since rolesFileContent already corresponds to a deserialized yaml representation of the roles files,
// we just unmarshal from the yaml data.
func parseRolesFileContent(data []byte) (RolesFileContent, error) {
	var parsed RolesFileContent
	err := yaml.Unmarshal(data, &parsed)
	return parsed, err
}

// fileBytes returns the file representation of rolesFileContent.
// Since rolesFileContent already corresponds to a deserialized yaml representation of the roles files,
// we just marshal it back to yaml.
func (r RolesFileContent) FileBytes() ([]byte, error) {
	return yaml.Marshal(&r)
}

// mergeWith merges multiple rolesFileContent, giving priority to other returning a new RolesFileContent.
func (r RolesFileContent) MergeWith(other RolesFileContent) RolesFileContent {
	result := make(RolesFileContent)
	for roleName, roleSpec := range r {
		result[roleName] = roleSpec
	}
	for roleName, roleSpec := range other {
		result[roleName] = roleSpec
	}
	return result
}

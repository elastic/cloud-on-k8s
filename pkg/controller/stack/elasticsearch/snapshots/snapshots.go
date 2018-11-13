package snapshots

import "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"

const (
	ServiceAccountFileName = "service-account.json"
)

// RepositoryCredentialsKey returns a provider specific keystore key for the corresponding credentials.
func RepositoryCredentialsKey(repoConfig v1alpha1.SnapshotRepository) string {
	switch repoConfig.Type {
	case "gcs":
		//TODO randomize name to avoid collisions with user created repos
		return "gcs.client.elastic-internal.credentials_file"

	}
	return ""
}

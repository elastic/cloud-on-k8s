package snapshots

import (
	"encoding/json"
	"github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/pkg/errors"
)

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

func validateGcsKeyFile(fileName string, credentials map[string]string) error {
	expected := []string{
		"type",
		"project_id",
		"private_key_id",
		"private_key",
		"client_email",
		"client_id",
		"auth_uri",
		"token_uri",
		"auth_provider_x509_cert_url",
		"client_x509_cert_url",
	}
	var missing []string
	var result error
	for _, k := range expected {
		if _, ok := credentials[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		result = errors.Errorf("Expected keys %v not found in %s gcs credential file", missing, fileName)
	}
	return result

}

// ValidateSnapshotCredentials does a superficial inspection of provider specific snapshot credentials typically retrieved via a secret.
func ValidateSnapshotCredentials(kind v1alpha1.SnapshotRepositoryType, raw map[string][]byte) error {
	switch kind {
	case v1alpha1.SnapshotRepositoryTypeGCS:
		var errs []error
		for k, v := range raw {
			var parsed map[string]string
			err := json.Unmarshal(v, &parsed)
			if err != nil {
				errs = append(errs, errors.Wrap(err, "gcs secrets need to be JSON, PKCS12 is not supported"))
				continue
			}
			if err := validateGcsKeyFile(k, parsed); err != nil {
				errs = append(errs, err)
			}
		}
		return common.NewCompoundError(errs)

	default:
		return errors.New(common.Concat("Unsupported snapshot repository type ", string(kind)))
	}
}

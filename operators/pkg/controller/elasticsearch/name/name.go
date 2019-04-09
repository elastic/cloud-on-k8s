package name

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("name")
)

const (
	// Dear developer, please do not exceed 27 characters or I will panic.
	PodSuffix                 = "-es"
	ConfigSecretSuffix        = "-config"
	ServiceSuffix             = "-es"
	DiscoveryServiceSuffix    = "-es-discovery"
	CASecretSuffix            = "-ca"
	CAPrivateKeySecretSuffix  = "-ca-private-key"
	ElasticUserSecretSuffix   = "-elastic-user"
	EsRolesUsersSecretSuffix  = "-es-roles-users"
	ExtraFilesSecretSuffix    = "-extrafiles"
	InternalUsersSecretSuffix = "-internal-users"
	KeystoreSuffix            = "-keystore"

	// Whatever the named resource, it must never exceed 63 characters to be used as a label.
	MaxLabelLength = 63
	// Elasticsearch name, used as prefix, is limited to 36 characters,
	MaxNameLength = 36
	// so it let 27 characters for a suffix.
	MaxSuffixLength = MaxLabelLength - MaxNameLength
)

// Suffix the Elasticsearch name.
// Panic if the name or the suffix exceeds the limits below.
func Suffix(name string, suffix string) string {
	if len(suffix) > MaxSuffixLength {
		panic(fmt.Errorf("suffix should not exceed %d characters: %s", MaxSuffixLength, suffix))
	}
	if len(name) > MaxNameLength {
		panic(fmt.Errorf("name should not exceed %d characters: %s", MaxNameLength, name))
	}
	return stringsutil.Concat(name, suffix)
}

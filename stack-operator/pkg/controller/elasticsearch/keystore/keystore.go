package keystore

import v1 "k8s.io/api/core/v1"

// Config contains all configuration to initialise a ES keystore
type Config struct {
	KeystoreSecretRef v1.SecretReference
	KeystoreSettings  []Setting
}

// IsEmpty is true when Config does not contain any config.
func (c Config) IsEmpty() bool {
	return len(c.KeystoreSettings) == 0
}

// Setting captures settings to be added to the keystore on init.
type Setting struct {
	Key           string
	ValueFilePath string
}

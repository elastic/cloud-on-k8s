package keystore

import "k8s.io/api/core/v1"

// Config contains all configuration to initialise a ES keystore
type Config struct {
	KeystoreSecretRef v1.SecretReference
	KeystoreSettings  []Setting
}

func (c Config) IsEmpty() bool {
	return len(c.KeystoreSettings) == 0
}

// Setting captures settings to be added to the keystore on init.
type Setting struct {
	Key           string
	ValueFilePath string
}

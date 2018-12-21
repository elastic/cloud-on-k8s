package keystore

const (
	ManagedSecretName = "keystore-secret"
	// SecretMountPath Mountpath for keystore secrets in init container.
	SecretMountPath = "/keystore-secrets"
	// SecretVolumeName is the the name of the volume where the keystore secret is referenced.
	SecretVolumeName = "keystore-init"
)

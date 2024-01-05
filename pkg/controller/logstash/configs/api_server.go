package configs

// APIServer hold the resolved api.* config
type APIServer struct {
	SSLEnabled       string
	KeystorePassword string
	AuthType         string
	Username         string
	Password         string
}

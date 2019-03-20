package certinitializer

import (
	"os"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("certificate-initializer")

type CertInitializer struct {
	config     Config
	CSR        []byte
	Terminated bool
}

func NewCertInitializer(cfg Config) CertInitializer {
	return CertInitializer{
		config:     cfg,
		Terminated: false,
	}
}

// execute the main program (see README.md for details).
func (i *CertInitializer) Start(http bool) error {
	if checkExistingOnDisk(i.config) {
		log.Info("Reusing existing private key, CSR and certificate")
		return nil
	}

	log.Info("Creating a private key on disk")
	privateKey, err := createAndStorePrivateKey(i.config.PrivateKeyPath)
	if err != nil {
		return err
	}

	log.Info("Generating a CSR from the private key")
	csr, err := createCSR(privateKey)
	if err != nil {
		return err
	}

	i.CSR = csr

	if http {
		log.Info("Serving CSR over HTTP", "port", i.config.Port)
		stopChan := make(chan struct{})
		defer close(stopChan)
		go func() {
			err := serveCSR(i.config.Port, csr, stopChan)
			//exitOnErr(err) FIXME
			if err != nil {
				log.Error(err, "Fail to serve CSR")
				os.Exit(1)
			}
		}()
	}

	log.Info("Watching filesystem for cert update")
	err = i.watchForCertUpdate()
	if err != nil {
		return err
	}

	log.Info("Certificate initialization successful")
	return nil
}

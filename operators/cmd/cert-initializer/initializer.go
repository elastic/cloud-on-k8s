package main

import (
	"os"
)

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

// Start executes the main program (see README.md for details).
func (i *CertInitializer) Start() error {
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

	log.Info("Serving CSR over HTTP", "port", i.config.Port)
	stopChan := make(chan struct{})
	defer close(stopChan)
	go func() {
		err := i.serveCSR(stopChan)
		if err != nil {
			log.Error(err, "Fail to serve CSR")
			os.Exit(1)
		}
	}()

	log.Info("Watching filesystem for cert update")
	err = i.watchForCertUpdate()
	if err != nil {
		return err
	}

	log.Info("Certificate initialization successful")
	return nil
}

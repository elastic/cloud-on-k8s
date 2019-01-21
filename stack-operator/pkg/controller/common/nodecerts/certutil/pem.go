package certutil

import (
	"crypto/x509"
	"encoding/pem"
)

// ParsePEMCerts returns a list of certificates from the given PEM certs data
// Based on the code of x509.CertPool.AppendCertsFromPEM (https://golang.org/src/crypto/x509/cert_pool.go)
// We don't rely on x509.CertPool.AppendCertsFromPEM directly here since it returns an interface from which
// we cannot extract the actual certificates if we need to compare them.
func ParsePEMCerts(pemData []byte) ([]*x509.Certificate, error) {
	certs := []*x509.Certificate{}
	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}

		certs = append(certs, cert)
	}
	return certs, nil
}

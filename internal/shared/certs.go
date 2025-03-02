package shared

import (
	"crypto/x509"
	"errors"
	"os"
)

////////////////////////////////////////////////////////////////////////////////

func CACertPool(caCertFn string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(caCertFn)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("failed to add certificate to pool")
	}

	return certPool, nil
}

////////////////////////////////////////////////////////////////////////////////

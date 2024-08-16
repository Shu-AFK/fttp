package helper

import (
	"crypto/tls"
	"fmt"
	"os"
)

func LoadCertificates(certPath string, keyPath string) ([]tls.Certificate, error) {
	certPEMBlock, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cert-PEMBlock certificate: %w", err)
	}
	keyPEMBlock, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key-PEMBlock certificate: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	return []tls.Certificate{cert}, nil
}

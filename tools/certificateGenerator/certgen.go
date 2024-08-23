package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

func main() {
	org := flag.String("org", "", "Organization name")
	commonName := flag.String("cn", "", "Common name (domain)")
	organizationalUnit := flag.String("on", "", "Organizational unit")
	ip := flag.String("ip", "", "IP address")
	hostName := flag.String("name", "", "Host name. Files will be saved as {name}-key.pem and {name}-cert.pem")
	flag.Parse()

	if *org == "" || *commonName == "" || *ip == "" || *organizationalUnit == "" || *hostName == "" {
		flag.Usage()
		os.Exit(1)
	}

	var notBefore time.Time
	notBefore = time.Now()

	validFor := 365 * 24 * time.Hour

	notAfter := notBefore.Add(validFor)

	key, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		os.Exit(1)
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	if err != nil {
		os.Exit(1)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{*org},
			OrganizationalUnit: []string{*organizationalUnit},
			CommonName:         *commonName,
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP(*ip)},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)

	if err != nil {
		os.Exit(1)
	}

	certOut, _ := os.Create(*hostName + "-cert.pem")

	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	err = certOut.Close()
	if err != nil {
		return
	}

	keyOut, _ := os.OpenFile(*hostName+"-key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)

	privBytes, _ := x509.MarshalPKCS8PrivateKey(key)

	err = pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	if err != nil {
		fmt.Println("error writing private key: %w", err)
	}

	err = keyOut.Close()
	if err != nil {
		fmt.Println("error closing private key file: %w", err)
	}
}

package httpmin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func selfSignedCertificate() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),

		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature,

		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},

		DNSNames: []string{
			"localhost",
		},

		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		&tmpl,
		&tmpl,
		&priv.PublicKey,
		priv,
	)

	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func certificateFromFiles(certFile, keyFile string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(certFile, keyFile)
}

// If cert.pem and key.pem exist, use those. Otherwise, generate new cert.
func selfSignedFromFolder(path string) (tls.Certificate, error) {
	certFile := filepath.Join(path, "cert.pem")
	keyFile := filepath.Join(path, "key.pem")

	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)

	// If both files exist, use them
	if certErr == nil && keyErr == nil {
		return certificateFromFiles(certFile, keyFile)
	}

	cert, err := selfSignedCertificate()

	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})

	privKey, ok := cert.PrivateKey.(*rsa.PrivateKey)

	if !ok {
		return tls.Certificate{}, errors.New("private key is not RSA")
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	err = os.MkdirAll(path, 0755)

	if err != nil {
		return tls.Certificate{}, err
	}

	err = os.WriteFile(certFile, certPEM, 0644)

	if err != nil {
		return tls.Certificate{}, err
	}

	err = os.WriteFile(keyFile, keyPEM, 0600) // owner read/write only

	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

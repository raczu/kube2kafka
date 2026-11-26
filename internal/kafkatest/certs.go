package kafkatest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// ClientCommonName is the subject common name of the client certificate from GenerateCerts.
const ClientCommonName = "kube2kafka-test-client"

// Certs holds PEM-encoded test certificates: a CA, plus a server certificate for
// localhost and 127.0.0.1 and a client certificate, both signed by that CA.
type Certs struct {
	CA         []byte
	ServerCert []byte
	ServerKey  []byte
	ClientCert []byte
	ClientKey  []byte
}

// GenerateCerts creates a fresh CA with a server and a client certificate. Every call
// creates a new CA, so certificates from two calls do not trust each other.
func GenerateCerts() (*Certs, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}
	caTemplate := template(1, "kube2kafka-test-ca")
	caTemplate.IsCA = true
	caTemplate.BasicConstraintsValid = true
	caTemplate.KeyUsage = x509.KeyUsageCertSign
	caDER, err := x509.CreateCertificate(
		rand.Reader,
		caTemplate,
		caTemplate,
		&caKey.PublicKey,
		caKey,
	)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	server := template(2, "localhost")
	server.DNSNames = []string{"localhost"}
	server.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
	server.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	serverCert, serverKey, err := issue(server, ca, caKey)
	if err != nil {
		return nil, fmt.Errorf("issue server certificate: %w", err)
	}

	client := template(3, ClientCommonName)
	client.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	clientCert, clientKey, err := issue(client, ca, caKey)
	if err != nil {
		return nil, fmt.Errorf("issue client certificate: %w", err)
	}

	return &Certs{
		CA:         pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		ServerCert: serverCert,
		ServerKey:  serverKey,
		ClientCert: clientCert,
		ClientKey:  clientKey,
	}, nil
}

// template returns a certificate template; serial must be unique per issuing CA.
func template(serial int64, commonName string) *x509.Certificate {
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: commonName},
		// Backdated so a slightly skewed clock does not reject a fresh certificate.
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}
}

// issue signs tmpl with the CA and returns the certificate and its key as PEM.
func issue(
	tmpl, ca *x509.Certificate,
	caKey *ecdsa.PrivateKey,
) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

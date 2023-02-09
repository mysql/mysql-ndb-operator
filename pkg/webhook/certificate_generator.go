// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	klog "k8s.io/klog/v2"
)

const (
	defaultCertValidity = 365 * 24 * time.Hour
)

// createCertificate creates a new X.509v3 certificate for the given service
func createCertificate(k8sServiceName, namespace string) *tlsData {
	// Generate serial number between 1 and 2^128 - 1 (max of 128 bits)
	maxSerialNumber := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	serialNumber, err := rand.Int(rand.Reader, maxSerialNumber)
	if err != nil {
		klog.Error("Failed to generate a random serial number : ", err)
		return nil
	}

	// Set time limits for the certificate
	notBefore := time.Now()
	notAfter := notBefore.Add(defaultCertValidity)

	// Create a certificate request template
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "ndb-operator-webhook-cert",
			Organization: []string{"Oracle Corporation"},
		},
		DNSNames: []string{
			k8sServiceName,
			k8sServiceName + "." + namespace + ".svc",
		},

		NotBefore: notBefore,
		NotAfter:  notAfter,

		// DigitalSignature and KeyEncipherment usages are required for RSA keys
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageKeyEncipherment |
			// certificate is also CA, as it self signs
			x509.KeyUsageCertSign,

		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Generate the private key to sign the certificate
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		klog.Error("Failed to generate client private key : ", err)
		return nil
	}

	// Create the certificate
	certificate, err := x509.CreateCertificate(rand.Reader, &template, &template, privateKey.Public(), privateKey)
	if err != nil {
		klog.Error("Failed to create certificate : ", err)
		return nil
	}

	// PEM encode certificate, private key and return
	certPEMBuffer := new(bytes.Buffer)
	err = pem.Encode(certPEMBuffer, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate,
	})
	if err != nil {
		klog.Error("Failed to PEM encode certificate : ", err)
		return nil
	}

	privateKeyPEMBuffer := new(bytes.Buffer)
	err = pem.Encode(privateKeyPEMBuffer, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err != nil {
		klog.Error("Failed to PEM encode private key : ", err)
		return nil
	}

	klog.Info("Certificate has been generated and encoded")

	return &tlsData{
		certificate: certPEMBuffer.Bytes(),
		privateKey:  privateKeyPEMBuffer.Bytes(),
	}
}

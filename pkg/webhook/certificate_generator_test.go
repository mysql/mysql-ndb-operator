// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func verifyHostname(t *testing.T, cert *x509.Certificate, hostname string) {
	t.Helper()
	if err := cert.VerifyHostname(hostname); err != nil {
		t.Fatal("Failed to verify hostname : ", err)
	}
}

func Test_createCertificate(t *testing.T) {
	// create the certificate and use it to verify itself
	td := createCertificate("test-service", "test-ns")

	if td == nil || td.certificate == nil || td.privateKey == nil {
		t.Fatal("newSelfSignedCertificate failed to create a new certificate")
	}

	// try creating a certificate
	_, err := tls.X509KeyPair(td.certificate, td.privateKey)
	if err != nil {
		t.Fatal("Failed to create certificate : ", err)
	}

	// verify that the certificate has the correct DNS names
	block, _ := pem.Decode(td.certificate)
	if block == nil {
		t.Fatalf("Failed to decode the generated certificate")
		// return to suppress incorrect staticcheck warnings for SA5011
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal("Failed to parse the generated certificate : ", err)
	}
	verifyHostname(t, cert, "test-service.test-ns.svc")
	verifyHostname(t, cert, "test-service")
}

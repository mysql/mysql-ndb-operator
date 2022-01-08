// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/mysql/ndb-operator/pkg/controllers"
	"github.com/mysql/ndb-operator/pkg/helpers"

	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
)

const (
	webHookServerAddr = ":9443"
)

// tlsData holds the pem encoded certificate and privateKey
type tlsData struct {
	certificate, privateKey []byte
}

// sendAdmissionResponse sends an AdmissionResponse wrapped in a AdmissionReview back to the caller
func sendAdmissionResponse(w http.ResponseWriter, response *v1.AdmissionResponse) {

	klog.V(2).Infof("Sending response: %s", response.String())

	// wrap it in a AdmissionReview and send it back
	responseAdmissionReview := v1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: response,
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(responseAdmissionReview)
	if err != nil {
		klog.Error(err)
	}
}

// serve handles the http portion of a request and then validates the review request
func serve(w http.ResponseWriter, r *http.Request, vtor validator) {

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		errMessage := fmt.Sprintf("Expected 'application/json' contentType but got '%s'", contentType)
		sendAdmissionResponse(w, requestDeniedBad("", errMessage))
		return
	}

	// Read all the request body content
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		sendAdmissionResponse(w, requestDeniedBad("", err.Error()))
		return
	}
	klog.V(5).Infof("Received body : %s", string(bodyBytes))

	// Decode the request body content into the AdmissionReview struct
	requestedAdmissionReview := v1.AdmissionReview{}
	_, _, err = scheme.Codecs.UniversalDeserializer().Decode(bodyBytes, nil, &requestedAdmissionReview)
	if err != nil {
		sendAdmissionResponse(w, requestDeniedBad("", err.Error()))
		return
	}
	klog.V(5).Infof("Received review request : %s", requestedAdmissionReview.String())

	if requestedAdmissionReview.APIVersion != "admission.k8s.io/v1" {
		// Only v1 is supported
		errMessage := fmt.Sprintf(
			"Webhook can only handle API version 'admission.k8s.io/v1' but got '%s'",
			requestedAdmissionReview.APIVersion)
		sendAdmissionResponse(w, requestDeniedBad("", errMessage))
		return
	}

	request := requestedAdmissionReview.Request
	if request == nil {
		sendAdmissionResponse(w, requestDeniedBad("", "Received an empty(nil) AdmissionRequest"))
		return
	}

	klog.Infof("Serving request with UID '%s'", request.UID)

	// Review the request and reply
	response := validate(request, vtor)
	sendAdmissionResponse(w, response)
}

// initWebhookServer sets up the handler and initializes the server
func initWebhookServer(ws *http.Server) {
	// set server address
	ws.Addr = webHookServerAddr

	// pattern to validator mapping
	validators := map[string]validator{
		"/validate-ndb": newNdbValidator(),
	}

	// setup the handler
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		// Handle readiness probe
		if request.URL.Path == "/health" {
			klog.V(2).Infof("Replying OK to health probe")
			writer.WriteHeader(http.StatusOK)
			return
		}

		// retrieve the validator for the pattern and send it to serve
		if vtor := validators[request.URL.Path]; vtor == nil {
			// No validator found. Handle error
			klog.V(2).Infof("No validator mapped for pattern '%s'", request.URL.Path)
			sendAdmissionResponse(writer, requestDeniedBad("", "requested URL path not found"))
		} else {
			// Found a validator for the pattern
			serve(writer, request, vtor)
		}
	})
}

// setWebhookServerTLSCerts configures the server to use the TLS certificates
func setWebhookServerTLSCerts(ws *http.Server) {
	namespace, err := helpers.GetCurrentNamespace()
	if err != nil {
		klog.Fatalf("Could not get current namespace : %s", err)
	}

	// Create a new certificate
	td := createCertificate(config.serviceName, namespace)

	// get k8s clientset
	clientset := getK8sClientset()
	if clientset == nil {
		klog.Fatal("Failed to create k8s clientset")
	}

	// update the webhook with the certificate
	vwcInterface := controllers.NewValidatingWebhookConfigController(clientset)
	if !vwcInterface.UpdateWebhookConfigCertificate(
		context.Background(), "webhook-server="+namespace+"-"+config.serviceName, td.certificate) {
		klog.Fatal("Failed to update validating webhook configs with the new certificate")
	}

	// Add certificate to server config
	cert, err := tls.X509KeyPair(td.certificate, td.privateKey)
	if err != nil {
		klog.Fatal(err)
	}
	ws.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}

// Run sets up a webhook server and handles incoming connections over TLS
// This is the main function of this package
func Run() {
	// Parse the arguments
	flag.Parse()
	validateCommandLineArgs()

	// init the server
	ws := &http.Server{}
	initWebhookServer(ws)

	// Setup TLS certificates
	setWebhookServerTLSCerts(ws)

	klog.Info("Webhook server : setup successful")

	// start the server
	klog.Info("Starting to serve...")
	if err := ws.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
		klog.Errorf("Error during listen and server : %v", err)
	} else {
		klog.Info("Webhook server : closed")
	}
}

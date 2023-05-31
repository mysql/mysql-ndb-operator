// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mysql/ndb-operator/pkg/controllers"
	"github.com/mysql/ndb-operator/pkg/helpers"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
)

const (
	webHookServerAddr = ":9443"

	// Note: when the operator is installed using OLM, OLM will install the
	// certificate in the below mentioned hardcoded path
	certFile = "tmp/k8s-webhook-server/serving-certs/tls.crt"
	keyFile  = "tmp/k8s-webhook-server/serving-certs/tls.key"
)

// tlsData holds the pem encoded certificate and privateKey
type tlsData struct {
	certificate, privateKey []byte
}

// sendAdmissionResponse sends an AdmissionResponse wrapped in a AdmissionReview back to the caller
func sendAdmissionResponse(w http.ResponseWriter, response *admissionv1.AdmissionResponse) {

	klog.V(2).Infof("Sending response: %s", response.String())

	// wrap it in a AdmissionReview and send it back
	responseAdmissionReview := admissionv1.AdmissionReview{
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

func sendAdmissionResponsePathNotFound(w http.ResponseWriter) {
	sendAdmissionResponse(w, requestDeniedBad("", "requested URL path not found"))
}

// serve handles the http portion of a request and then validates the review request
func serve(w http.ResponseWriter, r *http.Request, ac admissionController, executorFunc requestExecutor) {

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		errMessage := fmt.Sprintf("Expected 'application/json' contentType but got '%s'", contentType)
		sendAdmissionResponse(w, requestDeniedBad("", errMessage))
		return
	}

	// Read all the request body content
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		sendAdmissionResponse(w, requestDeniedBad("", err.Error()))
		return
	}
	klog.V(5).Infof("Received body : %s", string(bodyBytes))

	// Decode the request body content into the AdmissionReview struct
	requestedAdmissionReview := admissionv1.AdmissionReview{}
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
	response := executorFunc(request, ac)
	sendAdmissionResponse(w, response)
}

// initWebhookServer sets up the handler and initializes the server
func initWebhookServer(ws *http.Server) {
	// set server address
	ws.Addr = webHookServerAddr

	// setup the handlers
	http.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		// Handle readiness probe
		klog.V(2).Infof("Replying OK to health probe")
		writer.WriteHeader(http.StatusOK)
	})

	// pattern to admissionController mapping
	admissionControllers := map[string]admissionController{
		"ndb": newNdbAdmissionController(),
	}

	// allowed admissionController requestTypes
	validRequestTypes := map[string]requestExecutor{
		"validate": validate,
		"mutate":   mutate,
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		urlTokens := strings.Split(request.URL.Path[1:], "/")

		if len(urlTokens) == 2 {
			ac := admissionControllers[urlTokens[0]]
			executeFunc, isValidRequestType := validRequestTypes[urlTokens[1]]
			if ac != nil && isValidRequestType {
				// Found an admissionController for the pattern
				serve(writer, request, ac, executeFunc)
				return
			}
		}

		// No admissionController found or invalid requestType
		// Handle error
		klog.V(2).Infof("URL path not found '%s'", request.URL.Path)
		sendAdmissionResponsePathNotFound(writer)
	})
}

// setWebhookServerTLSCerts configures the server to use the TLS certificates
func setWebhookServerTLSCerts(ctx context.Context, ws *http.Server) {
	namespace, err := helpers.GetCurrentNamespace()
	if err != nil {
		klog.Fatalf("Could not get current namespace : %s", err)
	}

	var cert tls.Certificate

	// Check if the certificate and key files exist
	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)

	if certErr != nil || keyErr != nil {
		// Either certificate or key file is missing, generate new certificate
		td := createCertificate(config.serviceName, namespace)

		// Get k8s clientset
		clientset := getK8sClientset()
		if clientset == nil {
			klog.Fatal("Failed to create k8s clientset")
		}

		// Update the validating webhook config with the certificate
		vwcInterface := controllers.NewValidatingWebhookConfigController(clientset)
		if !vwcInterface.UpdateWebhookConfigCertificate(
			ctx, "webhook-server="+namespace+"-"+config.serviceName, td.certificate) {
			klog.Fatal("Failed to update validating webhook configs with the new certificate")
		}

		// Update the mutating webhook config with the certificate
		mwcInterface := controllers.NewMutatingWebhookConfigController(clientset)
		if !mwcInterface.UpdateWebhookConfigCertificate(
			ctx, "webhook-server="+namespace+"-"+config.serviceName, td.certificate) {
			klog.Fatal("Failed to update mutating webhook configs with the new certificate")
		}

		// Load the TLS certificate and key
		cert, err = tls.X509KeyPair(td.certificate, td.privateKey)
		if err != nil {
			klog.Fatal(err)
		}

	} else {
		// Load the TLS certificate and key files
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			klog.Fatal("Failed to load TLS certificate and key:", err)
		}
	}

	// Add certificate to server config
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
	setWebhookServerTLSCerts(context.Background(), ws)

	klog.Info("Webhook server : setup successful")

	// start the server
	klog.Info("Starting to serve...")
	if err := ws.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
		klog.Errorf("Error during listen and server : %v", err)
	} else {
		klog.Info("Webhook server : closed")
	}
}

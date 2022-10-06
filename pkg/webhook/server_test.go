// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const serverAddr = "http://localhost:9443/"

func prettifyStruct(v interface{}) string {
	if v == nil || reflect.ValueOf(v).IsNil() {
		return "nil"
	}
	res, _ := json.MarshalIndent(v, "", "  ")
	return "\n" + string(res)
}

type content struct {
	// Only one of this is needed
	// The first one set will be used as the testcase request body
	text string
	req  *v1.AdmissionRequest
}

type testCase struct {
	desc            string
	action          string
	contentType     string
	body            *content
	allowed         bool
	statusReason    metav1.StatusReason
	messageContains string
}

func (tc *testCase) getBodyReader() io.Reader {
	// Build and return a io.Reader
	var bodyReader io.Reader
	if tc.body != nil {
		if len(tc.body.text) > 0 {
			// create a reader reading from text
			bodyReader = strings.NewReader(tc.body.text)
		} else if tc.body.req != nil {
			// Set UID for the request
			tc.body.req.UID = "server-test"
			// wrap it in AdmissionReview and convert it
			review := &v1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Request:  tc.body.req,
				Response: nil,
			}
			reqBytes, _ := json.Marshal(review)
			bodyReader = bytes.NewReader(reqBytes)
		}
	}
	return bodyReader
}

func (tc *testCase) doPost(t *testing.T) *v1.AdmissionResponse {
	t.Helper()
	url := serverAddr + tc.action
	response, err := http.Post(url, tc.contentType, tc.getBodyReader())
	if err != nil {
		t.Errorf("action %s failed : %v", tc.action, err)
		return nil
	}

	if response.StatusCode != 200 {
		t.Errorf("Server returned error for action '%s' : %v", tc.action, response.Status)
		return nil
	}

	// Decode the response body into the AdmissionReview struct
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Errorf("Failed to read http response : %v", err)
		return nil
	}
	responseAdmissionReview := v1.AdmissionReview{}
	_, _, err = scheme.Codecs.UniversalDeserializer().Decode(bodyBytes, nil, &responseAdmissionReview)
	if err != nil {
		t.Logf("Response : %v", response.Body)
		t.Errorf("Failed to decode response as AdmissionReview : %v", err)
		return nil
	}

	return responseAdmissionReview.Response
}

func (tc *testCase) expectRequestAllowed(t *testing.T) {
	t.Helper()
	response := tc.doPost(t)
	if !response.Allowed {
		t.Errorf("Request was expected to be allowed but was not allowed.")
		t.Errorf("Response recieved : %s", prettifyStruct(response))
	}
}

func (tc *testCase) expectRequestNotAllowed(t *testing.T) {
	t.Helper()
	response := tc.doPost(t)
	if response == nil {
		return
	}

	if response.Allowed {
		t.Errorf("Request was expected not to be allowed but was allowed : %v", response)
		return
	}

	result := response.Result
	if result.Reason != tc.statusReason ||
		result.Status != metav1.StatusFailure ||
		!strings.Contains(result.Message, tc.messageContains) {
		t.Errorf("Unexpected result received : %s", prettifyStruct(result))
		t.Errorf("Expected : status : '%s' reason : '%s' message : '%s'",
			metav1.StatusFailure, tc.statusReason, tc.messageContains)
	}
}

func Test_serve(t *testing.T) {

	testcases := []testCase{
		{
			// Test invalid validator pattern
			desc:            "invalid pattern",
			action:          "dummy",
			allowed:         false,
			statusReason:    metav1.StatusReasonBadRequest,
			messageContains: "requested URL path not found",
		},
		{
			// Test invalid content type
			desc:            "invalid content type",
			action:          "ndb/validate",
			contentType:     "application/example",
			allowed:         false,
			statusReason:    metav1.StatusReasonBadRequest,
			messageContains: "Expected 'application/json' contentType but got 'application/example'",
		},
		{
			// Test a non JSON content
			desc:   "invalid JSON content",
			action: "ndb/validate",
			body: &content{
				text: "not a json content",
			},
			allowed:         false,
			statusReason:    metav1.StatusReasonBadRequest,
			messageContains: "cannot unmarshal string into Go value of type struct",
		},
		{
			// Test bad JSON
			desc:   "unsupported JSON array",
			action: "ndb/validate",
			body: &content{
				text: "[]",
			},
			allowed:         false,
			statusReason:    metav1.StatusReasonBadRequest,
			messageContains: "cannot unmarshal array into Go value of type struct",
		},
		{
			// Test bad AdmissionReview version
			desc:   "bad AdmissionReview version",
			action: "ndb/validate",
			body: &content{
				text: func() string {
					review := &v1beta1.AdmissionReview{
						TypeMeta: metav1.TypeMeta{
							Kind:       "AdmissionReview",
							APIVersion: "admission.k8s.io/v1beta1",
						},
					}
					reqBytes, _ := json.Marshal(review)
					return string(reqBytes)
				}(),
			},
			allowed:         false,
			statusReason:    metav1.StatusReasonBadRequest,
			messageContains: "Webhook can only handle API version 'admission.k8s.io/v1' but got 'admission.k8s.io/v1beta1'",
		},
		{
			// Test empty admission request
			desc:   "empty admission request",
			action: "ndb/validate",
			body: &content{
				text: func() string {
					review := &v1.AdmissionReview{
						TypeMeta: metav1.TypeMeta{
							Kind:       "AdmissionReview",
							APIVersion: "admission.k8s.io/v1",
						},
						Request: nil,
					}
					reqBytes, _ := json.Marshal(review)
					return string(reqBytes)
				}(),
			},
			allowed:         false,
			statusReason:    metav1.StatusReasonBadRequest,
			messageContains: "Received an empty(nil) AdmissionRequest",
		},
	}

	// Run the testcases
	for _, tc := range testcases {
		// Fill up defaults
		if len(tc.contentType) == 0 {
			tc.contentType = "application/json"
		}

		t.Logf("Testing %s", tc.desc)
		if tc.allowed {
			tc.expectRequestAllowed(t)
		} else {
			tc.expectRequestNotAllowed(t)
		}
	}
}

func Test_probes(t *testing.T) {
	// Test the liveness/readiness probes reply
	tc := &testCase{
		// Test probe reply
		action: "health",
	}

	t.Logf("Testing probe replies")
	// Should get a OK reply
	tc.doPost(t)
}

func TestMain(m *testing.M) {
	// Create and init a webhook server
	server := &http.Server{}
	initWebhookServer(server)

	// Use a channel to wait for server shutdown in the end
	listenAndServeErr := make(chan error, 1)
	go func() {
		// Listen and server http requests. As the test is
		// concerned only about the functioning of the actual
		// server, http is sufficient.
		listenAndServeErr <- server.ListenAndServe()
	}()

	// run the tests
	code := m.Run()

	// stop server
	if err := server.Shutdown(context.Background()); err != nil {
		log.Fatalf("Failed to shutdown : %v", err)
	}

	// wait for it to close
	if err := <-listenAndServeErr; err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe failed : %v", err)
	}

	os.Exit(code)
}

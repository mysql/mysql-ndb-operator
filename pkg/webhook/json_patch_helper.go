// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"encoding/json"
)

// jsonPatchOperations holds all the json operations
// that will be returned by the mutator
type jsonPatchOperations struct {
	ops []map[string]interface{}
}

// empty returns true if the ops array is empty
func (jpo *jsonPatchOperations) empty() bool {
	return jpo.ops == nil
}

// JSONPath operation types
const (
	ADD     = "add"
	REPLACE = "replace"
)

// newOp appends a new operation to the ops array
func (jpo *jsonPatchOperations) newOp(operation, path string, value interface{}) {
	jpo.ops = append(jpo.ops, map[string]interface{}{
		"op":    operation,
		"path":  path,
		"value": value,
	})
}

// add appends an ADD operation to the ops array
func (jpo *jsonPatchOperations) add(path string, value interface{}) {
	jpo.newOp(ADD, path, value)
}

// replace appends a REPLACE operation to the ops array
func (jpo *jsonPatchOperations) replace(path string, value interface{}) {
	jpo.newOp(REPLACE, path, value)
}

// getPatch returns the operations as a json patch
func (jpo *jsonPatchOperations) getPatch() ([]byte, error) {
	if jpo.empty() {
		return nil, nil
	}
	return json.Marshal(jpo.ops)
}

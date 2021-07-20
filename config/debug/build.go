// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package debug

import (
	"errors"
	"fmt"
)

// Panic panics in debug builds
func Panic(v interface{}) {
	if Enabled {
		panic(v)
	}
}

// InternalError panics in a debug build and returns an error in a release build
func InternalError(v interface{}) error {
	errMsg := fmt.Sprintf("internal error : %v", v)
	if Enabled {
		panic(errMsg)
	} else {
		return errors.New(errMsg)
	}
}

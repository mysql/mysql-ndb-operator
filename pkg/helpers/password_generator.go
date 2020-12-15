// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"math/rand"
	"time"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateRandomPassword generates a random alpha numeric password of length n
func GenerateRandomPassword(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[random.Int63()%int64(len(letters))]
	}
	return string(b)
}

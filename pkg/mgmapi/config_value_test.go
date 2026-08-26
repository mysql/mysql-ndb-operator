// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import "testing"

func TestConfigValueTypeValidation(t *testing.T) {
	expectedUint32 := uint32(42)
	actualUint32, err := configValueAsUint32(expectedUint32, sysCfgConfigGenerationNumber)
	if err != nil {
		t.Fatalf("expected uint32 conversion to succeed: %v", err)
	}
	if actualUint32 != expectedUint32 {
		t.Fatalf("expected uint32 value %d, got %d", expectedUint32, actualUint32)
	}

	expectedUint64 := uint64(1) << 40
	actualUint64, err := configValueAsUint64(expectedUint64, dbCfgDataMemory)
	if err != nil {
		t.Fatalf("expected uint64 conversion to succeed: %v", err)
	}
	if actualUint64 != expectedUint64 {
		t.Fatalf("expected uint64 value %d, got %d", expectedUint64, actualUint64)
	}

	// Wrong wire types must return errors instead of reaching type assertions.
	if _, err := configValueAsUint32(uint64(1), sysCfgConfigGenerationNumber); err == nil {
		t.Fatal("expected uint32 conversion to reject a uint64 value")
	}
	if _, err := configValueAsUint64(uint32(1), dbCfgDataMemory); err == nil {
		t.Fatal("expected uint64 conversion to reject a uint32 value")
	}
}

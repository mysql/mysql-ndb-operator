// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import "fmt"

func configValueAsUint32(value configValue, configKey uint32) (uint32, error) {
	typedValue, ok := value.(uint32)
	if !ok {
		// Config entry types are supplied by the management reply and need validation.
		return 0, fmt.Errorf("config key %d has unexpected value type %T", configKey, value)
	}
	return typedValue, nil
}

func configValueAsUint64(value configValue, configKey uint32) (uint64, error) {
	typedValue, ok := value.(uint64)
	if !ok {
		// Config entry types are supplied by the management reply and need validation.
		return 0, fmt.Errorf("config key %d has unexpected value type %T", configKey, value)
	}
	return typedValue, nil
}

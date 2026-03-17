package parser

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/enbility/ship-go/ship"
)

// NormalizeEEBUSJSON converts EEBUS-specific JSON encoding to standard JSON.
// EEBUS uses arrays of single-key objects instead of standard JSON objects.
// Also strips null bytes appended by some devices.
func NormalizeEEBUSJSON(data []byte) []byte {
	// Strip null bytes
	data = bytes.TrimRight(data, "\x00")
	if len(data) == 0 {
		return data
	}
	// Use ship-go's helper for the array-to-object conversion
	normalized := ship.JsonFromEEBUSJson(data)
	return normalized
}

// fixupSliceFields wraps non-array values in arrays when the target struct
// field expects a slice. EEBUS JSON sometimes omits the array brackets for
// single-element arrays.
func fixupSliceFields(data []byte, target interface{}) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return data
	}

	targetType := reflect.TypeOf(target)
	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}
	if targetType.Kind() != reflect.Struct {
		return data
	}

	changed := false
	for i := 0; i < targetType.NumField(); i++ {
		field := targetType.Field(i)
		if field.Type.Kind() != reflect.Slice {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		jsonName := strings.Split(tag, ",")[0]
		val, ok := raw[jsonName]
		if !ok || len(val) == 0 {
			continue
		}
		trimmed := bytes.TrimSpace(val)
		if len(trimmed) > 0 && trimmed[0] != '[' {
			raw[jsonName] = json.RawMessage("[" + string(val) + "]")
			changed = true
		}
	}

	if !changed {
		return data
	}

	result, err := json.Marshal(raw)
	if err != nil {
		return data
	}
	return result
}

package common

import "encoding/json"

type CommonConfig struct {
	SignalAddr string
}

// RemarshalPayload converts a generic payload (typically from map[string]interface{})
// into a specific struct type using JSON marshaling and unmarshaling.
func RemarshalPayload[T any](payload interface{}) (T, error) {
	var target T
	data, err := json.Marshal(payload)
	if err != nil {
		return target, err
	}
	err = json.Unmarshal(data, &target)
	return target, err
}

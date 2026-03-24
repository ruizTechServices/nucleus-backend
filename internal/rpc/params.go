package rpc

import "encoding/json"

func DecodeParams[T any](raw json.RawMessage) (T, error) {
	var params T
	if len(raw) == 0 {
		return params, nil
	}

	if err := json.Unmarshal(raw, &params); err != nil {
		return params, err
	}

	return params, nil
}

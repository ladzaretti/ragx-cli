package vecdb

import "encoding/json"

type Meta struct {
	Source string `json:"path,omitempty"`
	Index  int    `json:"index,omitempty"`
}

func DecodeMeta(raw json.RawMessage) (Meta, error) {
	var m Meta
	if len(raw) == 0 {
		return m, nil
	}

	err := json.Unmarshal(raw, &m)

	return m, err
}

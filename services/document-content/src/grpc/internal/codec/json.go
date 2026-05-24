package codec

import "encoding/json"

type JSONCodec struct{}

func (JSONCodec) Name() string { return "json" }

func (JSONCodec) Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (JSONCodec) Unmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

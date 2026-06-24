package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSON []byte

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid json value: %s", string(j))
	}
	return string(j), nil
}

func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}

	switch data := value.(type) {
	case []byte:
		*j = append((*j)[0:0], data...)
	case string:
		*j = append((*j)[0:0], data...)
	default:
		return fmt.Errorf("unsupported json scan type %T", value)
	}
	return nil
}

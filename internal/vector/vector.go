package vector

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
)

type Vector []float32

func New(values []float32) Vector {
	if values == nil {
		return nil
	}
	cloned := make(Vector, len(values))
	copy(cloned, values)
	return cloned
}

func (v Vector) Slice() []float32 {
	if v == nil {
		return nil
	}
	cloned := make([]float32, len(v))
	copy(cloned, v)
	return cloned
}

func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	encoded, err := json.Marshal([]float32(v))
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

func (v *Vector) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}

	var raw []byte
	switch typed := src.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	default:
		return fmt.Errorf("vector: unsupported scan type %T", src)
	}

	var decoded []float32
	if len(raw) == 0 {
		*v = nil
		return nil
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*v = Vector(decoded)
	return nil
}

type Int64Slice []int64

func (s Int64Slice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	encoded, err := json.Marshal([]int64(s))
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

func (s *Int64Slice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}

	var raw []byte
	switch typed := src.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	default:
		return fmt.Errorf("int64 slice: unsupported scan type %T", src)
	}

	var decoded []int64
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*s = Int64Slice(decoded)
	return nil
}

func CosineSimilarity(left, right []float32) float32 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}

	var dot float64
	var leftNorm float64
	var rightNorm float64
	for index := range left {
		lv := float64(left[index])
		rv := float64(right[index])
		dot += lv * rv
		leftNorm += lv * lv
		rightNorm += rv * rv
	}

	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)))
}
package ai

import (
	"encoding/json"
	"errors"
)

func (shape *catalogShape) ensureOriginal() error {
	if shape.raw == nil {
		return nil
	}
	shape.originalOnce.Do(func() {
		pairs, err := orderedCatalogPairs(shape.raw)
		if err != nil {
			shape.originalErr = err
			return
		}
		shape.original = make(map[string]json.RawMessage, len(pairs))
		for _, pair := range pairs {
			if _, seen := shape.original[pair.key]; !seen {
				shape.order = append(shape.order, pair.key)
			}
			shape.original[pair.key] = pair.value
		}
	})
	return shape.originalErr
}

func (shape *catalogShape) ensureBaseline() error {
	if err := shape.ensureOriginal(); err != nil {
		return err
	}
	if shape.raw == nil {
		return nil
	}
	shape.baselineOnce.Do(func() {
		// Decode the immutable original, never the possibly edited live model.
		// Otherwise the first write could mistake a changed price for its baseline
		// and silently restore the old value.
		model, known, err := decodeCatalogTyped(shape.raw)
		if err != nil {
			shape.baselineErr = err
			return
		}
		if !known {
			shape.baselineErr = errors.New("decode stored model: unknown model type")
			return
		}
		record, _, err := anyModelRecord(model)
		if err != nil {
			shape.baselineErr = err
			return
		}
		pairs, err := catalogRecordJSON(record)
		if err != nil {
			shape.baselineErr = err
			return
		}
		shape.baseline = make(map[string]json.RawMessage, len(pairs))
		for _, pair := range pairs {
			shape.baseline[pair.key] = pair.value
		}
	})
	return shape.baselineErr
}

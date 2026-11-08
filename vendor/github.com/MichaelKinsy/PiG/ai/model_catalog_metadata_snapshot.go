//go:build go1.27

package ai

import jsonv2 "encoding/json/v2"

// Decode the normal persisted schema directly, without allocating a map and
// boxed scalar for every row. JSON v2 matches field names case-sensitively and
// rejects duplicate keys. Noncanonical inputs use the existing validating
// decoder, preserving its last-key-wins and malformed-row recovery behavior.
type decodedCatalogSnapshot struct {
	Version   *float64                   `json:"version"`
	FetchedAt *float64                   `json:"fetchedAt"`
	CheckedAt *float64                   `json:"checkedAt"`
	ETag      string                     `json:"etag"`
	Rows      *[]decodedStoredCatalogRow `json:"rows"`
}

type decodedStoredCatalogRow struct {
	ModelCatalogMetadataRow
	Provider      *string  `json:"provider"`
	ID            *string  `json:"id"`
	ContextWindow *float64 `json:"contextWindow"`
	MaxTokens     *float64 `json:"maxTokens"`
}

func decodeTypedCatalogSnapshot(payload []byte) (ModelCatalogMetadataSnapshot, bool) {
	var decoded decodedCatalogSnapshot
	if err := jsonv2.Unmarshal(payload, &decoded); err != nil || decoded.Version == nil || (*decoded.Version != ModelCatalogMetadataVersion && *decoded.Version != 1) || decoded.FetchedAt == nil || decoded.CheckedAt == nil || decoded.Rows == nil {
		return ModelCatalogMetadataSnapshot{}, false
	}
	snapshot := ModelCatalogMetadataSnapshot{
		Version: int(*decoded.Version), FetchedAt: int64(*decoded.FetchedAt),
		CheckedAt: int64(*decoded.CheckedAt), ETag: decoded.ETag,
		Rows: make([]ModelCatalogMetadataRow, 0, len(*decoded.Rows)),
	}
	for _, decodedRow := range *decoded.Rows {
		row, valid := decodedRow.validated()
		if valid {
			snapshot.Rows = append(snapshot.Rows, row)
		}
	}
	return snapshot, true
}

func (decoded decodedStoredCatalogRow) validated() (ModelCatalogMetadataRow, bool) {
	if decoded.Provider == nil || decoded.ID == nil {
		return ModelCatalogMetadataRow{}, false
	}
	row := decoded.ModelCatalogMetadataRow
	row.Provider, row.ID = *decoded.Provider, *decoded.ID
	if row.TPS != nil && *row.TPS <= 0 {
		return ModelCatalogMetadataRow{}, false
	}
	for _, modality := range row.Input {
		if modality != ModelInputText && modality != ModelInputImage {
			return ModelCatalogMetadataRow{}, false
		}
	}
	if len(row.Input) == 0 {
		row.Input = nil
	}
	var efforts []string
	for _, effort := range row.Efforts {
		if effort == "none" {
			continue
		}
		known := false
		for _, tier := range modelCatalogEffortLevels {
			if effort == tier {
				known = true
				break
			}
		}
		if !known {
			return ModelCatalogMetadataRow{}, false
		}
		if efforts == nil {
			efforts = row.Efforts[:0]
		}
		efforts = append(efforts, effort)
	}
	row.Efforts = efforts
	if decoded.ContextWindow != nil {
		count := int(*decoded.ContextWindow)
		if count <= 0 {
			return ModelCatalogMetadataRow{}, false
		}
		row.ContextWindow = &count
	}
	if decoded.MaxTokens != nil {
		count := int(*decoded.MaxTokens)
		if count <= 0 {
			return ModelCatalogMetadataRow{}, false
		}
		row.MaxTokens = &count
	}
	return row, true
}

package modelroles

import "encoding/json"

// UnmarshalJSON tracks presence so a project can explicitly unhide or restore
// a global role without replacing its name/color. Recursive layering preserves
// authored false overrides for both visibility and deletion.
func (t *Tag) UnmarshalJSON(data []byte) error {
	type plain Tag
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*t = Tag(value)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, t.nameSet = fields["name"]
	_, t.colorSet = fields["color"]
	_, t.hiddenSet = fields["hidden"]
	_, t.deletedSet = fields["deleted"]
	return nil
}

// MarshalJSON retains authored false/empty overrides while omitting absent fields.
func (t Tag) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if t.nameSet || t.Name != "" {
		fields["name"] = t.Name
	}
	if t.colorSet || t.Color != "" {
		fields["color"] = t.Color
	}
	if t.hiddenSet || t.Hidden {
		fields["hidden"] = t.Hidden
	}
	if t.deletedSet || t.Deleted {
		fields["deleted"] = t.Deleted
	}
	return json.Marshal(fields)
}

// WithDeleted authors a deletion/restoration marker without losing other tag
// fields. Explicit false must survive serialization to restore a lower layer.
func (t Tag) WithDeleted(deleted bool) Tag {
	t.Deleted, t.deletedSet = deleted, true
	return t
}

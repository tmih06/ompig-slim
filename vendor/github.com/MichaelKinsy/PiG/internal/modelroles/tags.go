package modelroles

import "encoding/json"

// UnmarshalJSON tracks presence so a project can explicitly unhide a global tag
// without replacing its name/color, matching OMP's recursive record layering.
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
	return json.Marshal(fields)
}

package modelroles

// Info carries OMP's built-in tag, friendly name, semantic color, and section.
// Custom metadata overrides only authored fields; built-in tag identities stay stable.
type Info struct {
	Tag, Name, Color string
	Kind             bool
	Hidden           bool
}

// RoleInfo is the shared display metadata for built-in and user-defined roles.
func (c Config) RoleInfo(role string) Info {
	info := Info{Name: role, Color: "muted"}
	switch role {
	case "default":
		info = Info{Tag: "DEFAULT", Name: "Default", Color: "success"}
	case "smol":
		info = Info{Tag: "SMOL", Name: "Fast", Color: "warning"}
	case "slow":
		info = Info{Tag: "SLOW", Name: "Thinking", Color: "accent"}
	case "vision":
		info = Info{Tag: "VISION", Name: "Vision", Color: "error"}
	case "plan":
		info = Info{Tag: "PLAN", Name: "Architect", Color: "muted"}
	case "commit":
		info = Info{Tag: "COMMIT", Name: "Commit", Color: "dim"}
	case "tiny":
		info = Info{Tag: "TINY", Name: "Tiny", Color: "dim"}
	case "memory":
		info = Info{Tag: "MEMORY", Name: "Memory", Color: "dim"}
	case "task":
		info = Info{Tag: "TASK", Name: "Subtask", Color: "muted"}
	case "advisor":
		info = Info{Tag: "ADVISOR", Name: "Advisor", Color: "accent"}
	case "widget":
		info = Info{Tag: "WIDGET", Name: "Job widgets", Color: "dim"}
	case "compact":
		info = Info{Tag: "COMPACT", Name: "Compaction", Color: "dim"}
	case "image":
		info = Info{Tag: "IMAGE", Name: "Image generation", Color: "accent", Kind: true}
	case "web":
		info = Info{Tag: "WEB", Name: "Web search", Color: "success", Kind: true}
	case "speech":
		info = Info{Tag: "SPEECH", Name: "Speech", Color: "warning", Kind: true}
	case "dictation":
		info = Info{Tag: "DICTATION", Name: "Dictation", Color: "warning", Kind: true}
	case "judge":
		info = Info{Tag: "JUDGE", Name: "Judge", Color: "muted", Kind: true}
	}
	if tag, ok := c.ModelTags[role]; ok {
		if tag.Name != "" {
			info.Name = tag.Name
		}
		if tag.Color != "" {
			info.Color = tag.Color
		}
		info.Hidden = tag.Hidden
	}
	return info
}

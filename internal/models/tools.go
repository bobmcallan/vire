package models

// ToolDefinition describes an MCP tool and its HTTP mapping for dynamic registration.
type ToolDefinition struct {
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Method      string            `json:"method"`
	Params      []ParamDefinition `json:"params"`
}

// ParamDefinition describes a single parameter for a tool.
type ParamDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	In          string `json:"in"`
	DefaultFrom string `json:"default_from,omitempty"`
}

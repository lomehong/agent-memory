package plugin

import "encoding/json"

// OpenClawToolSchema defines the tool schema for OpenClaw plugin integration.
// These schemas describe the tools that agent-memory exposes to OpenClaw agents.

// ToolSchemas is the list of tool schemas for OpenClaw integration.
var ToolSchemas = []ToolSchema{
	{
		Name:        "memory_store",
		Description: "存储一条长期记忆，自动去重和分类",
		Parameters: ToolParams{
			Type: "object",
			Properties: map[string]ParamSpec{
				"content": {
					Type:        "string",
					Description: "记忆内容",
					Required:    true,
				},
				"category": {
					Type:        "string",
					Description: "分类: identity/principle/knowledge/working（可选，自动推断）",
					Enum:        []string{"identity", "principle", "knowledge", "working"},
				},
				"visibility": {
					Type:        "string",
					Description: "可见性: private/team/user（可选，自动推断）",
					Enum:        []string{"private", "team", "user"},
				},
				"priority": {
					Type:        "number",
					Description: "优先级 0.0-1.0（可选，自动推断）",
				},
				"tags": {
					Type:        "array",
					Description: "标签列表",
					Items:       "string",
				},
			},
			Required: []string{"content"},
		},
	},

	{
		Name:        "memory_search",
		Description: "语义检索记忆",
		Parameters: ToolParams{
			Type: "object",
			Properties: map[string]ParamSpec{
				"query": {
					Type:        "string",
					Description: "搜索查询",
					Required:    true,
				},
				"category": {
					Type:        "string",
					Description: "按分类过滤",
					Enum:        []string{"identity", "principle", "knowledge", "working"},
				},
				"limit": {
					Type:        "number",
					Description: "返回数量限制（默认10）",
				},
			},
			Required: []string{"query"},
		},
	},

	{
		Name:        "memory_list",
		Description: "列出记忆（支持分类和状态过滤）",
		Parameters: ToolParams{
			Type: "object",
			Properties: map[string]ParamSpec{
				"category": {
					Type:        "string",
					Description: "按分类过滤",
					Enum:        []string{"identity", "principle", "knowledge", "working"},
				},
				"status": {
					Type:        "string",
					Description: "按状态过滤",
					Enum:        []string{"active", "degraded", "archived"},
				},
				"limit": {
					Type:        "number",
					Description: "返回数量限制（默认20）",
				},
			},
			Required: []string{},
		},
	},

	{
		Name:        "memory_forget",
		Description: "删除一条记忆",
		Parameters: ToolParams{
			Type: "object",
			Properties: map[string]ParamSpec{
				"memory_id": {
					Type:        "string",
					Description: "记忆ID",
					Required:    true,
				},
			},
			Required: []string{"memory_id"},
		},
	},

	{
		Name:        "memory_report",
		Description: "获取记忆健康报告",
		Parameters: ToolParams{
			Type:       "object",
			Properties: map[string]ParamSpec{},
			Required:   []string{},
		},
	},
}

// ToolSchema represents an OpenClaw tool definition.
type ToolSchema struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  ToolParams `json:"parameters"`
}

// ToolParams represents tool parameter schema.
type ToolParams struct {
	Type       string                 `json:"type"`
	Properties map[string]ParamSpec   `json:"properties"`
	Required   []string               `json:"required"`
}

// ParamSpec represents a single parameter specification.
type ParamSpec struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"-"`
	Enum        []string `json:"enum,omitempty"`
	Items       string   `json:"items,omitempty"`
}

// GetToolSchemasJSON returns all tool schemas as JSON.
func GetToolSchemasJSON() (string, error) {
	data, err := json.MarshalIndent(ToolSchemas, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HandleToolCall routes an OpenClaw tool call to the appropriate handler.
// Returns the result as a JSON string.
func HandleToolCall(toolName string, params json.RawMessage, handler func(name string, params json.RawMessage) (interface{}, error)) (string, error) {
	result, err := handler(toolName, params)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

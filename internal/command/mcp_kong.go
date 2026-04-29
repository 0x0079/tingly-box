//go:build !legacy

package command

// MCPBuiltinCmdKong starts the builtin MCP server. Registered at the top level
// as "mcp-builtin" to match the legacy command path, which is consumed by
// internal/mcp/runtime/builtin_registry.go.
type MCPBuiltinCmdKong struct{}

func (m *MCPBuiltinCmdKong) Run(appManager *AppManager) error {
	cmd := MCPBuiltinCommand()
	return cmd.Execute()
}

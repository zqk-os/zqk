package main

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/mcp"
)

func TestMCPToolsRegistration(t *testing.T) {
	server := mcp.NewServer()
	registerMinimalTools(server)

	// Ensure zqk_object_list is registered
	tools := server.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "zqk_object_list" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("zqk_object_list not found")
	}
}

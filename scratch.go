package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
)

func main() {
	prompt, _ := os.ReadFile("/tmp/zqk-execute-prompt.txt")

	payload := map[string]interface{}{
		"model": "zqk-qwen:latest",
		"messages": []map[string]interface{}{
			{"role": "user", "content": string(prompt)},
		},
		"tools": []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "dummy_tool",
					"description": "Dummy tool",
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	resp, _ := http.Post("http://localhost:11434/v1/chat/completions", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	res, _ := io.ReadAll(resp.Body)
	os.WriteFile("/tmp/zqk-curl-output-dummy.json", res, 0644)
}

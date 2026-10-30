package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Rafix4x/chatgpt-api/internal/chatgpt"
)

type openAIMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []openAITool    `json:"tools"`
}

func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
		return sb.String()
	}
	return ""
}

func toolsToPrompt(tools []openAITool) string {
	var defs []map[string]any
	for _, t := range tools {
		if t.Function.Name == "" {
			continue
		}
		var params any
		if len(t.Function.Parameters) > 0 {
			_ = json.Unmarshal(t.Function.Parameters, &params)
		}
		defs = append(defs, map[string]any{
			"name":        t.Function.Name,
			"description": t.Function.Description,
			"parameters":  params,
		})
	}
	if len(defs) == 0 {
		return ""
	}
	raw, _ := json.MarshalIndent(defs, "", "  ")
	return "You can call these tools:\n" + string(raw) +
		"\n\nTo call a tool answer only with JSON: {\"tool_calls\":[{\"name\":\"<name>\",\"arguments\":{...}}]}"
}

func estimateTokens(msgs []chatgpt.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)/4 + 4
	}
	return n
}

func parseToolCalls(text string) []map[string]any {
	var envelope struct {
		ToolCalls []struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"tool_calls"`
	}
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if json.Unmarshal([]byte(trimmed), &envelope) != nil || len(envelope.ToolCalls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(envelope.ToolCalls))
	for i, tc := range envelope.ToolCalls {
		out = append(out, map[string]any{
			"id":    "call_" + randHex(8),
			"type":  "function",
			"index": i,
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": string(tc.Arguments),
			},
		})
	}
	return out
}

func (a *App) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "invalid JSON"}})
		return
	}

	modelID, slug := chatgpt.ResolveModel(req.Model)

	msgs := make([]chatgpt.Message, 0, len(req.Messages)+1)
	for _, m := range req.Messages {
		text := flattenContent(m.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		msgs = append(msgs, chatgpt.Message{Role: m.Role, Content: text})
	}
	if len(msgs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "empty prompt"}})
		return
	}
	if tp := toolsToPrompt(req.Tools); tp != "" {
		msgs = append([]chatgpt.Message{{Role: "system", Content: tp}}, msgs...)
	}

	cid := "chatcmpl-" + randHex(12)
	created := time.Now().Unix()

	if req.Stream {
		if !startSSE(w) {
			return
		}
		writeChunk := func(delta map[string]any, finish any) {
			_ = writeSSEData(w, map[string]any{
				"id":      cid,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   modelID,
				"choices": []map[string]any{{
					"index":         0,
					"delta":         delta,
					"finish_reason": finish,
				}},
			})
		}

		writeChunk(map[string]any{"role": "assistant", "content": ""}, nil)

		err := a.Chat.GenerateStream(chatgpt.ChatRequest{Model: slug, Messages: msgs}, func(delta string) error {
			writeChunk(map[string]any{"content": delta}, nil)
			return nil
		})
		if err != nil {
			a.Logf("chatgpt stream error: %v", err)
			writeChunk(map[string]any{"content": "\n[upstream error: " + err.Error() + "]"}, nil)
		}
		writeChunk(map[string]any{}, "stop")
		_ = writeSSEDone(w)
		return
	}

	text, err := a.Chat.Generate(chatgpt.ChatRequest{Model: slug, Messages: msgs})
	if err != nil {
		status := http.StatusBadGateway
		if ue, ok := err.(*chatgpt.UpstreamError); ok && ue.Status == http.StatusTooManyRequests {
			status = http.StatusTooManyRequests
		}
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}
	if text == "" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": "upstream error: empty completion"}})
		return
	}

	message := map[string]any{"role": "assistant", "content": text}
	choice := map[string]any{"index": 0, "message": message, "finish_reason": "stop"}
	if len(req.Tools) > 0 {
		if calls := parseToolCalls(text); len(calls) > 0 {
			message["tool_calls"] = calls
			choice["finish_reason"] = "tool_calls"
		}
	}

	promptTokens := estimateTokens(msgs)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      cid,
		"object":  "chat.completion",
		"created": created,
		"model":   modelID,
		"choices": []map[string]any{choice},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": len(text) / 4,
			"total_tokens":      promptTokens + len(text)/4,
		},
	})
}

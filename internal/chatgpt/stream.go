package chatgpt

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

type Event struct {
	MessageID string
	Created   bool
	Done      bool
	Text      string
	Title     string
	Err       error
}

type sseMessage struct {
	ID     string `json:"id"`
	Author struct {
		Role string `json:"role"`
	} `json:"author"`
	Content struct {
		ContentType string   `json:"content_type"`
		Parts       []string `json:"parts"`
	} `json:"content"`
	Recipient   string          `json:"recipient"`
	Status      string          `json:"status"`
	Error       string          `json:"error"`
	MessageType string          `json:"message_type"`
	Metadata    json.RawMessage `json:"metadata"`
}

type sseFrame struct {
	Message        *sseMessage `json:"message"`
	ConversationID string      `json:"conversation_id"`
	Error          string      `json:"error"`
	Detail         string      `json:"detail"`
	Type           string      `json:"type"`
}

func ParseSSE(r io.Reader, emit func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var event string
	var deltaEnc string
	var full strings.Builder
	var titleSent bool

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
			continue
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(line[len("data:"):])
			if data == "" || data == "[DONE]" {
				continue
			}

			switch event {
			case "delta_encoding":
				_ = json.Unmarshal([]byte(data), &deltaEnc)
				continue
			case "delta", "":
				if strings.Contains(data, `"p":`) && strings.Contains(data, `"o":`) {
					var patch struct {
						P string          `json:"p"`
						O string          `json:"o"`
						V json.RawMessage `json:"v"`
					}
					if err := json.Unmarshal([]byte(data), &patch); err == nil {
						if patch.O == "append" && strings.Contains(patch.P, "/content/parts/0") {
							var s string
							if json.Unmarshal(patch.V, &s) == nil && s != "" {
								full.WriteString(s)
								if err := emit(Event{Text: s}); err != nil {
									return err
								}
							}
						}
						continue
					}
				}
			case "message":
			default:
				continue
			}

			var frame sseFrame
			if err := json.Unmarshal([]byte(data), &frame); err != nil {
				continue
			}

			if frame.Error != "" {
				return &UpstreamError{Kind: "upstream", Msg: frame.Error}
			}
			if frame.Detail != "" {
				return &UpstreamError{Kind: "upstream", Msg: frame.Detail}
			}
			if frame.Message == nil {
				continue
			}
			if frame.Message.Error != "" {
				return &UpstreamError{Kind: "upstream", Msg: frame.Message.Error}
			}

			ev := Event{MessageID: frame.Message.ID}
			if frame.Message.Author.Role == "assistant" && frame.Message.Recipient == "all" && !isCreateOnly(frame.Message.Metadata) {
				text := strings.Join(frame.Message.Content.Parts, "")
				if text == "" {
					continue
				}
				if len(text) > full.Len() && strings.HasPrefix(text, full.String()) {
					rest := text[full.Len():]
					full.WriteString(rest)
					if deltaEnc != "v1" {
						ev.Text = rest
					}
				}
				if frame.Message.Status == "finished_successfully" && !titleSent {
					if t := extractTitle(frame.Message.Metadata); t != "" {
						ev.Title = t
						titleSent = true
					}
				}
			}
			if ev.Text != "" || ev.Title != "" {
				if err := emit(ev); err != nil {
					return err
				}
			}
			continue
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return emit(Event{Done: true})
}

func isCreateOnly(metadata json.RawMessage) bool {
	if len(metadata) == 0 {
		return false
	}
	var m struct {
		IsComplete *bool `json:"is_complete"`
	}
	if json.Unmarshal(metadata, &m) != nil {
		return false
	}
	return m.IsComplete != nil && !*m.IsComplete
}

func extractTitle(metadata json.RawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var m struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(metadata, &m) != nil {
		return ""
	}
	return m.Title
}

func SplitText(parts []string) string {
	return strings.Join(parts, "")
}

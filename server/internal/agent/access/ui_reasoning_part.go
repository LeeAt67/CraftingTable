package access

import "encoding/json"

type uiReasoningStartPart struct {
	ID string
}

func (uiReasoningStartPart) UIStreamType() string {
	return "reasoning-start"
}

func (p uiReasoningStartPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"reasoning-start", p.ID})
}

type uiReasoningDeltaPart struct {
	ID    string
	Delta string
}

func (uiReasoningDeltaPart) UIStreamType() string {
	return "reasoning-delta"
}

func (p uiReasoningDeltaPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Delta string `json:"delta"`
	}{"reasoning-delta", p.ID, p.Delta})
}

type uiReasoningEndPart struct {
	ID string
}

func (uiReasoningEndPart) UIStreamType() string {
	return "reasoning-end"
}

func (p uiReasoningEndPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"reasoning-end", p.ID})
}

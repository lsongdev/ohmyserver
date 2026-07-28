package terminal

import (
	"encoding/json"
	"fmt"
)

type clientMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

type serverMessage struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

func parseClientMessage(data []byte) (clientMessage, error) {
	var message clientMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return message, fmt.Errorf("decode terminal control: %w", err)
	}
	if message.Type != "resize" {
		return message, fmt.Errorf("unknown terminal control %q", message.Type)
	}
	if message.Cols == 0 || message.Rows == 0 {
		return message, fmt.Errorf("terminal size must be positive")
	}
	return message, nil
}

func encodeServerMessage(message serverMessage) []byte {
	data, _ := json.Marshal(message)
	return data
}

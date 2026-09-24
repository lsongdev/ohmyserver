package terminal

import "testing"

func TestParseResizeMessage(t *testing.T) {
	message, err := parseClientMessage([]byte(`{"type":"resize","cols":120,"rows":40}`))
	if err != nil {
		t.Fatal(err)
	}
	if message.Cols != 120 || message.Rows != 40 {
		t.Fatalf("unexpected size: %dx%d", message.Cols, message.Rows)
	}
}

func TestParseResizeMessageRejectsInvalidControl(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"type":"unknown"}`),
		[]byte(`{"type":"resize","cols":0,"rows":24}`),
		[]byte(`not-json`),
	}
	for _, data := range tests {
		if _, err := parseClientMessage(data); err == nil {
			t.Fatalf("expected %q to fail", data)
		}
	}
}

package acme

import (
	"context"
	"testing"
)

func TestEngineRejectsIssuanceAfterClose(t *testing.T) {
	engine := NewEngine(context.Background(), nil, nil, nil)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StartIssuance(context.Background(), IssuanceRequest{}); err == nil {
		t.Fatal("StartIssuance succeeded after Engine.Close")
	}
}

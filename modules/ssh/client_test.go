package ssh

import (
	"context"
	"errors"
	"testing"
)

func TestConnectHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Connect(ctx, Profile{Host: "127.0.0.1", Port: 22, Username: "test", AuthType: "password", Password: "x"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect error = %v, want context canceled", err)
	}
}
func TestConnectRejectsUnsupportedAuthentication(t *testing.T) {
	_, err := Connect(context.Background(), Profile{Host: "127.0.0.1", Port: 22, Username: "test", AuthType: "unsupported"})
	if err == nil {
		t.Fatal("Connect accepted unsupported authentication type")
	}
}

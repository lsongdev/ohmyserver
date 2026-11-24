package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseLinks(t *testing.T) {
	text := "trojan://secret@example.com:443?sni=tls.example.com#Node\n" +
		"vless://00048082-0907-4b2e-b8a8-e0ecfe53ec06@example.net:443?security=tls&type=ws&path=%2Fproxy#VLESS\n" +
		"ss://" + base64.RawStdEncoding.EncodeToString([]byte("aes-128-gcm:secret")) + "@example.org:8388#SS\nunsupported://secret@example.com\n"
	for _, body := range []string{text, base64.StdEncoding.EncodeToString([]byte(text))} {
		nodes, warnings, err := Parse([]byte(body))
		if err != nil || len(nodes) != 3 || len(warnings) != 1 {
			t.Fatalf("parse nodes=%d warnings=%d error=%v", len(nodes), len(warnings), err)
		}
		for _, node := range nodes {
			if strings.Contains(node.Name, "secret") {
				t.Fatal("credential leaked in display name")
			}
		}
		var ob struct {
			Settings       struct{ Password string }
			StreamSettings struct{ Security string }
		}
		_ = json.Unmarshal(nodes[0].Outbound, &ob)
		if ob.Settings.Password != "secret" || ob.StreamSettings.Security != "tls" {
			t.Fatal("Trojan settings lost")
		}
	}
}
func TestMalformedSS(t *testing.T) {
	for _, s := range []string{"ss://", "ss://" + base64.RawStdEncoding.EncodeToString([]byte("example.com:80")), "ss://bad@example.com:80"} {
		if _, _, err := Parse([]byte(s)); err == nil {
			t.Fatalf("accepted malformed link")
		}
	}
}

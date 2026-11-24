// Package subscription converts proxy subscription documents to Xray outbounds.
package subscription

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Node struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Protocol string          `json:"protocol"`
	Address  string          `json:"address"`
	Outbound json.RawMessage `json:"outbound"`
}

func decode(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(strings.TrimSpace(s)); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

func Parse(body []byte) ([]Node, []string, error) {
	text := strings.TrimSpace(string(body))
	if !strings.Contains(text, "://") && !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		b, err := decode(text)
		if err != nil {
			return nil, nil, fmt.Errorf("expected proxy links or Xray JSON")
		}
		text = string(b)
	}
	var outbounds []json.RawMessage
	if strings.HasPrefix(text, "{") {
		var doc struct {
			Outbounds []json.RawMessage `json:"outbounds"`
		}
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			return nil, nil, fmt.Errorf("invalid Xray JSON")
		}
		outbounds = doc.Outbounds
	} else if strings.HasPrefix(text, "[") {
		if err := json.Unmarshal([]byte(text), &outbounds); err != nil {
			return nil, nil, fmt.Errorf("invalid outbound array")
		}
	} else {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			outbound, err := parseLink(line)
			if err != nil {
				outbounds = append(outbounds, nil)
				continue
			}
			raw, _ := json.Marshal(outbound)
			outbounds = append(outbounds, raw)
		}
	}
	if len(outbounds) > 2000 {
		return nil, nil, fmt.Errorf("subscription exceeds 2000 entries")
	}
	nodes := []Node{}
	warnings := []string{}
	seen := map[string]bool{}
	for i, raw := range outbounds {
		var doc map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &doc) != nil {
			warnings = append(warnings, fmt.Sprintf("Entry %d has an unsupported or invalid proxy link.", i+1))
			continue
		}
		protocol, _ := doc["protocol"].(string)
		name, _ := doc["tag"].(string)
		if protocol == "" {
			warnings = append(warnings, fmt.Sprintf("Entry %d has no protocol.", i+1))
			continue
		}
		delete(doc, "tag")
		canonical, _ := json.Marshal(doc)
		sum := sha256.Sum256(canonical)
		id := hex.EncodeToString(sum[:16])
		if seen[id] {
			continue
		}
		seen[id] = true
		settings, _ := doc["settings"].(map[string]any)
		address, _ := settings["address"].(string)
		if name == "" {
			name = address
		}
		if name == "" {
			name = protocol
		}
		nodes = append(nodes, Node{ID: id, Name: name, Protocol: protocol, Address: address, Outbound: canonical})
	}
	if len(nodes) == 0 {
		return nil, warnings, fmt.Errorf("no supported nodes found")
	}
	return nodes, warnings, nil
}

func parseLink(link string) (map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("invalid link")
	}
	protocol := u.Scheme
	if protocol == "ss" {
		return parseSS(u)
	}
	if protocol == "vmess" {
		return parseVMess(strings.TrimPrefix(link, "vmess://"))
	}
	if protocol != "trojan" && protocol != "vless" {
		return nil, fmt.Errorf("unsupported protocol")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Hostname() == "" || u.User == nil {
		return nil, fmt.Errorf("invalid endpoint")
	}
	q := u.Query()
	settings := map[string]any{"address": u.Hostname(), "port": port}
	if protocol == "trojan" {
		settings["password"] = u.User.Username()
	} else {
		settings["id"] = u.User.Username()
		settings["encryption"] = "none"
		if q.Get("encryption") != "" {
			settings["encryption"] = q.Get("encryption")
		}
		if q.Get("flow") != "" {
			settings["flow"] = q.Get("flow")
		}
	}
	stream, err := transport(q, protocol == "trojan")
	if err != nil {
		return nil, err
	}
	return map[string]any{"tag": u.Fragment, "protocol": protocol, "settings": settings, "streamSettings": stream}, nil
}

func transport(q url.Values, tlsDefault bool) (map[string]any, error) {
	network := q.Get("type")
	if network == "" {
		network = "raw"
	}
	if network == "tcp" {
		network = "raw"
	}
	stream := map[string]any{"network": network}
	switch network {
	case "raw":
	case "ws":
		stream["wsSettings"] = map[string]any{"path": q.Get("path"), "headers": map[string]string{"Host": q.Get("host")}}
	case "grpc":
		stream["grpcSettings"] = map[string]any{"serviceName": q.Get("serviceName")}
	case "httpupgrade":
		stream["httpupgradeSettings"] = map[string]any{"path": q.Get("path"), "host": q.Get("host")}
	case "xhttp":
		stream["xhttpSettings"] = map[string]any{"path": q.Get("path"), "host": q.Get("host"), "mode": q.Get("mode")}
	default:
		return nil, fmt.Errorf("unsupported transport")
	}
	security := q.Get("security")
	if security == "" && tlsDefault {
		security = "tls"
	}
	if security == "" {
		security = "none"
	}
	stream["security"] = security
	serverName := q.Get("sni")
	if serverName == "" {
		serverName = q.Get("peer")
	}
	switch security {
	case "none":
	case "tls":
		tls := map[string]any{"serverName": serverName}
		if q.Get("fp") != "" {
			tls["fingerprint"] = q.Get("fp")
		}
		if q.Get("alpn") != "" {
			tls["alpn"] = strings.Split(q.Get("alpn"), ",")
		}
		if q.Get("allowInsecure") == "1" || q.Get("allowInsecure") == "true" {
			tls["allowInsecure"] = true
		}
		stream["tlsSettings"] = tls
	case "reality":
		stream["realitySettings"] = map[string]any{"serverName": serverName, "fingerprint": q.Get("fp"), "password": q.Get("pbk"), "shortId": q.Get("sid"), "spiderX": q.Get("spx")}
	default:
		return nil, fmt.Errorf("unsupported security")
	}
	return stream, nil
}

func parseSS(u *url.URL) (map[string]any, error) {
	// SIP002 supports both base64 userinfo and legacy whole-authority encoding.
	if u.User == nil {
		b, err := decode(u.Host + u.Path)
		if err != nil {
			return nil, err
		}
		parsed, err := url.Parse("ss://" + string(b))
		if err != nil {
			return nil, err
		}
		parsed.Fragment = u.Fragment
		u = parsed
	}
	if u.User == nil {
		return nil, fmt.Errorf("invalid Shadowsocks credentials")
	}
	credential := u.User.String()
	if !strings.Contains(credential, ":") {
		b, err := decode(u.User.Username())
		if err != nil {
			return nil, err
		}
		credential = string(b)
	} else {
		credential, _ = url.PathUnescape(credential)
	}
	parts := strings.SplitN(credential, ":", 2)
	port, err := strconv.Atoi(u.Port())
	if len(parts) != 2 || err != nil || port < 1 || port > 65535 || u.Hostname() == "" || u.Query().Get("plugin") != "" {
		return nil, fmt.Errorf("invalid or unsupported Shadowsocks endpoint")
	}
	return map[string]any{"tag": u.Fragment, "protocol": "shadowsocks", "settings": map[string]any{"address": u.Hostname(), "port": port, "method": parts[0], "password": parts[1]}}, nil
}

func parseVMess(encoded string) (map[string]any, error) {
	b, err := decode(encoded)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil {
		return nil, fmt.Errorf("invalid VMess link")
	}
	str := func(key string) string {
		if v[key] == nil {
			return ""
		}
		return fmt.Sprint(v[key])
	}
	port, err := strconv.Atoi(str("port"))
	if err != nil || port < 1 || port > 65535 || str("add") == "" || str("id") == "" {
		return nil, fmt.Errorf("invalid VMess endpoint")
	}
	q := url.Values{"type": {str("net")}, "path": {str("path")}, "host": {str("host")}, "sni": {str("sni")}, "fp": {str("fp")}, "alpn": {str("alpn")}}
	if str("tls") == "tls" {
		q.Set("security", "tls")
	}
	if str("net") == "grpc" {
		q.Set("serviceName", str("path"))
	}
	if str("aid") != "" && str("aid") != "0" {
		return nil, fmt.Errorf("legacy VMess alterId unsupported")
	}
	stream, err := transport(q, false)
	if err != nil {
		return nil, err
	}
	security := str("scy")
	if security == "" {
		security = "auto"
	}
	return map[string]any{"tag": str("ps"), "protocol": "vmess", "settings": map[string]any{"address": str("add"), "port": port, "id": str("id"), "security": security}, "streamSettings": stream}, nil
}

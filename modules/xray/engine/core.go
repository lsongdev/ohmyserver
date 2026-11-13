// Package engine manages one embedded Xray instance and its saved configuration.
package engine

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
)

// compile uses Xray's own configuration builder without opening listeners.
func compile(raw json.RawMessage) (cfg *core.Config, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("Xray configuration: %v", v)
		}
	}()
	// Require strict JSON so the same document can be round-tripped by the UI.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("configuration must be a JSON object")
	}
	return serial.LoadJSONConfig(bytes.NewReader(raw))
}

func start(cfg *core.Config) (instance *core.Instance, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("Xray startup: %v", v)
		}
		if err != nil && instance != nil {
			_ = instance.Close()
			instance = nil
		}
	}()
	instance, err = core.New(cfg)
	if err != nil {
		return nil, err
	}
	err = instance.Start()
	return instance, err
}

package system

import (
	"context"
	"strings"
	"testing"
)

// Commands run in a UTF-8 locale: under C, nmcli turns a network name such
// as 一路高歌-5G into ????-5G.
func TestExecRunsCommandsInAUTF8Locale(t *testing.T) {
	out, _, err := Exec{}.Run(context.Background(), "", "sh", "-c", `printf %s "$LC_ALL"`)
	if err != nil || !strings.HasSuffix(string(out), "UTF-8") {
		t.Errorf("LC_ALL = %q, %v", out, err)
	}
}

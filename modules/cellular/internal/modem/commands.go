package modem

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/at"
)

type CommandResult struct {
	Lines []string `json:"lines"`
	Final string   `json:"final"`
}

type InvalidCommand string

func (e InvalidCommand) Error() string { return string(e) }

// Command sends a single line and returns the modem's response.
func (d Device) Command(ctx context.Context, command string) (CommandResult, error) {
	upper := strings.ToUpper(command)
	if !strings.HasPrefix(upper, "AT") || len(command) > 256 ||
		strings.ContainsFunc(command, func(r rune) bool { return r < ' ' || r > '~' || r == ';' }) {
		return CommandResult{}, InvalidCommand("Enter a single AT command of up to 256 printable characters, without semicolons.")
	}
	for _, prefix := range []string{"ATD", "ATO", "AT+CGDATA", "AT+CMGS", "AT+CMGW", "AT+CMGC"} {
		if strings.HasPrefix(upper, prefix) {
			return CommandResult{}, InvalidCommand("This command requires an interactive or data session.")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	lines, err := d.session.Query(ctx, command, func(string) bool { return true }, nil)
	result := CommandResult{Lines: lines, Final: "OK"}
	var rejection at.Rejection
	if errors.As(err, &rejection) {
		result.Final = rejection.Error()
		return result, nil
	}
	return result, err
}

func (m *Monitor) Command(ctx context.Context, command string) (CommandResult, error) {
	var result CommandResult
	err := m.execute(ctx, func(ctx context.Context, device *Device) (err error) {
		result, err = device.Command(ctx, command)
		return err
	})
	return result, err
}

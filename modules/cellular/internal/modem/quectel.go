// Package modem discovers Quectel modules and collects their cellular status.
package modem

import (
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/warthog618/sms"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/at"
)

type Cell struct {
	Radio string `json:"radio"`
	PLMN  string `json:"plmn,omitempty"`
	Band  *int   `json:"band,omitempty"`
	RSRP  *int   `json:"rsrp,omitempty"`
	RSRQ  *int   `json:"rsrq,omitempty"`
	SINR  *int   `json:"sinr,omitempty"`
}

type Reading struct {
	Model     string            `json:"model,omitempty"`
	Firmware  string            `json:"firmware,omitempty"`
	SIM       string            `json:"sim,omitempty"`
	Operator  *Operator         `json:"operator,omitempty"`
	Attached  *bool             `json:"packet_attached,omitempty"`
	LTE       *int              `json:"lte_registration,omitempty"`
	NR        *int              `json:"nr_registration,omitempty"`
	CellState string            `json:"cell_state,omitempty"`
	Cells     []Cell            `json:"cells,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// Transport handles complete AT replies and prompted submissions.
type Transport interface {
	Query(context.Context, string, func(string) bool, func(string)) ([]string, error)
	Submit(context.Context, string, string) ([]string, bool, error)
}

// Device exposes the modem operations for a single, serialized AT session.
type Device struct {
	session Transport
	encoder *sms.Encoder
}

// NewDevice wraps one AT session. Callers serialize operations on that session.
func NewDevice(session Transport) *Device {
	return &Device{session: session, encoder: sms.NewEncoder(sms.AsSubmit)}
}

type Operator struct {
	Name             string `json:"name,omitempty"`
	Mode             int    `json:"mode"`
	Format           *int   `json:"format,omitempty"`
	AccessTechnology *int   `json:"access_technology,omitempty"`
}

type InvalidReply string

func (e InvalidReply) Error() string { return "Unexpected reply to " + string(e) }

func (d Device) Model(ctx context.Context) (string, error) {
	return d.value(ctx, "ATI", "RM520")
}

func (d Device) Firmware(ctx context.Context) (string, error) {
	return d.value(ctx, "AT+CGMR", "RM520")
}

func (d Device) SIM(ctx context.Context) (string, error) {
	value, err := d.value(ctx, "AT+CPIN?", "+CPIN:")
	var rejected at.Rejection
	if errors.As(err, &rejected) {
		if state := simRejection(rejected); state != "" {
			return state, nil
		}
	}
	return value, err
}

func (d Device) LTERegistration(ctx context.Context) (*int, error) {
	return d.registration(ctx, "AT+CEREG?", "+CEREG:")
}

func (d Device) NRRegistration(ctx context.Context) (*int, error) {
	return d.registration(ctx, "AT+C5GREG?", "+C5GREG:")
}

func (d Device) registration(ctx context.Context, command, prefix string) (*int, error) {
	lines, err := d.query(ctx, command, func(line string) bool {
		return strings.HasPrefix(line, prefix) && registration(line) != nil
	})
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, InvalidReply(command)
	}
	return registration(lines[0]), nil
}

func (d Device) ServingCells(ctx context.Context) (string, []Cell, error) {
	lines, err := d.query(ctx, `AT+QENG="servingcell"`, prefixMatch("+QENG:"))
	if err != nil {
		return "", nil, err
	}
	state, cells := servingCells(lines)
	if state == "" {
		return "", nil, InvalidReply("AT+QENG")
	}
	return state, cells, nil
}

func (d Device) Operator(ctx context.Context) (*Operator, error) {
	const command = "AT+COPS?"
	lines, err := d.query(ctx, command, prefixMatch("+COPS:"))
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, InvalidReply(command)
	}
	values := fields(lines[0])
	mode := number(values, 0)
	if mode == nil {
		return nil, InvalidReply(command)
	}
	op := &Operator{Mode: *mode, Format: number(values, 1), AccessTechnology: number(values, 3)}
	if len(values) >= 3 && strings.ContainsFunc(values[2], func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		op.Name = values[2]
	}
	return op, nil
}

func (d Device) PacketAttached(ctx context.Context) (*bool, error) {
	value, err := d.value(ctx, "AT+CGATT?", "+CGATT:")
	if err != nil {
		return nil, err
	}
	if value != "0" && value != "1" {
		return nil, InvalidReply("AT+CGATT?")
	}
	attached := value == "1"
	return &attached, nil
}

func (d Device) value(ctx context.Context, command, prefix string) (string, error) {
	lines, err := d.query(ctx, command, prefixMatch(prefix))
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", InvalidReply(command)
	}
	if strings.HasSuffix(prefix, ":") {
		return payload(lines[0]), nil
	}
	return lines[0], nil
}

func prefixMatch(prefix string) func(string) bool {
	return func(line string) bool { return strings.HasPrefix(line, prefix) }
}

func (d Device) query(ctx context.Context, command string, match func(string) bool) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return d.session.Query(ctx, command, match, nil)
}

// Read collects independent fields. A rejected or malformed query affects
// only its field; a transport failure invalidates the session.
func (d Device) Read(ctx context.Context) (Reading, error) {
	r := Reading{Errors: make(map[string]string)}
	operations := []struct {
		key  string
		read func() error
	}{
		{"model", func() (err error) { r.Model, err = d.Model(ctx); return }},
		{"firmware", func() (err error) { r.Firmware, err = d.Firmware(ctx); return }},
		{"sim", func() (err error) { r.SIM, err = d.SIM(ctx); return }},
		{"lte_registration", func() (err error) { r.LTE, err = d.LTERegistration(ctx); return }},
		{"nr_registration", func() (err error) { r.NR, err = d.NRRegistration(ctx); return }},
		{"operator", func() (err error) { r.Operator, err = d.Operator(ctx); return }},
		{"packet_attached", func() (err error) { r.Attached, err = d.PacketAttached(ctx); return }},
		{"cells", func() (err error) { r.CellState, r.Cells, err = d.ServingCells(ctx); return }},
	}
	for _, operation := range operations {
		if err := operation.read(); err != nil {
			if !replyError(err) {
				return Reading{}, err
			}
			r.Errors[operation.key] = err.Error()
		}
	}
	return r, nil
}

func replyError(err error) bool {
	var rejected at.Rejection
	var malformed InvalidReply
	return errors.As(err, &rejected) || errors.As(err, &malformed)
}

func simRejection(err at.Rejection) string {
	switch strings.ToUpper(payload(err.Error())) {
	case "10", "SIM NOT INSERTED":
		return "SIM not inserted"
	case "11", "SIM PIN REQUIRED":
		return "SIM PIN required"
	case "12", "SIM PUK REQUIRED":
		return "SIM PUK required"
	case "13", "SIM FAILURE":
		return "SIM failure"
	default:
		return ""
	}
}

func payload(line string) string {
	_, value, _ := strings.Cut(line, ":")
	return strings.TrimSpace(value)
}

func fields(line string) []string {
	reader := csv.NewReader(strings.NewReader(payload(line)))
	reader.TrimLeadingSpace = true
	values, _ := reader.Read()
	return values
}

func registration(line string) *int {
	values := strings.SplitN(payload(line), ",", 3)
	if len(values) < 2 || number(values, 0) == nil {
		return nil
	}
	return number(values, 1)
}

func servingCells(lines []string) (string, []Cell) {
	var state string
	var cells []Cell
	for _, line := range lines {
		values := fields(line)
		if len(values) == 0 {
			continue
		}
		if values[0] == "servingcell" && len(values) >= 2 {
			state = values[1]
			values = values[2:]
		}
		if len(values) == 0 {
			continue
		}
		switch values[0] {
		case "NR5G-SA":
			cells = append(cells, Cell{Radio: "5G SA", PLMN: plmn(values, 2), Band: number(values, 8), RSRP: number(values, 10), RSRQ: number(values, 11), SINR: number(values, 12)})
		case "NR5G-NSA":
			// Firmware reports -32768 when the NR leg is unavailable.
			if rsrp := number(values, 4); rsrp != nil {
				cells = append(cells, Cell{Radio: "5G NSA", PLMN: plmn(values, 1), Band: number(values, 8), RSRP: rsrp, RSRQ: number(values, 6), SINR: number(values, 5)})
			}
		case "LTE":
			cells = append(cells, Cell{Radio: "LTE", PLMN: plmn(values, 2), Band: number(values, 7), RSRP: number(values, 11), RSRQ: number(values, 12), SINR: number(values, 14)})
		}
	}
	return state, cells
}

func number(values []string, i int) *int {
	if i >= len(values) {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(values[i]))
	if err != nil || n == -32768 || n == 65535 {
		return nil
	}
	return &n
}

// Keep the modem's leading zeroes in the mobile network code.
func plmn(values []string, i int) string {
	if len(values) <= i+1 {
		return ""
	}
	mcc, mnc := values[i], values[i+1]
	if len(mcc) != 3 || len(mnc) < 2 || len(mnc) > 3 || strings.Trim(mcc+mnc, "0123456789") != "" {
		return ""
	}
	return mcc + "-" + mnc
}

package modem

import (
	"context"
	"errors"
	"testing"
)

func TestCommandRejectsMultipleLines(t *testing.T) {
	device := NewDevice(nil)
	for _, command := range []string{"AT\rAT+QGPS=1", "AT;AT+QGPS=1", "ATD1234", "AT+CMGS=12"} {
		_, err := device.Command(context.Background(), command)
		var invalid InvalidCommand
		if !errors.As(err, &invalid) {
			t.Errorf("%q: expected invalid command error, got %v", command, err)
		}
	}
}

func TestCommandReturnsResponse(t *testing.T) {
	device := NewDevice(&smsTransport{responses: map[string][]string{"ATI": {"Quectel", "RM520N-GL"}}})
	reply, err := device.Command(context.Background(), "ATI")
	if err != nil || reply.Final != "OK" || len(reply.Lines) != 2 || reply.Lines[1] != "RM520N-GL" {
		t.Fatalf("reply=%+v, err=%v", reply, err)
	}
}

func TestOperatorReplacementCharacters(t *testing.T) {
	device := NewDevice(&smsTransport{responses: map[string][]string{
		"AT+COPS?":              {`+COPS: 0,0,"????",11`},
		`AT+QENG="servingcell"`: {`+QENG: "servingcell","NOCONN","NR5G-SA","TDD",460,01,000000001,1,00001,627264,78,3,-82,-11,16,1,32`},
	}})
	operator, err := device.Operator(context.Background())
	if err != nil || operator == nil || operator.Name != "" {
		t.Fatalf("operator=%+v, err=%v", operator, err)
	}
	_, cells, err := device.ServingCells(context.Background())
	if err != nil || len(cells) != 1 || cells[0].PLMN != "460-01" {
		t.Fatalf("cells=%+v, err=%v", cells, err)
	}
}

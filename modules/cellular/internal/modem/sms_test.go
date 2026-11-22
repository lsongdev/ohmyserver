package modem

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/at"
	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/pdumode"
	"github.com/warthog618/sms/encoding/tpdu"
)

type smsTransport struct {
	commands        []string
	responses       map[string][]string
	submissions     []*tpdu.TPDU
	loseSecondReply bool
}

func (s *smsTransport) Query(_ context.Context, command string, match func(string) bool, _ func(string)) ([]string, error) {
	s.commands = append(s.commands, command)
	var replies []string
	for _, line := range s.responses[command] {
		if match(line) {
			replies = append(replies, line)
		}
	}
	return replies, nil
}

func (s *smsTransport) Submit(_ context.Context, command, raw string) ([]string, bool, error) {
	pdu, err := pdumode.UnmarshalHexString(raw)
	if err != nil {
		return nil, false, err
	}
	if command != fmt.Sprintf("AT+CMGS=%d", len(pdu.TPDU)) {
		return nil, false, fmt.Errorf("unexpected command %s", command)
	}
	packet := &tpdu.TPDU{Direction: tpdu.MO}
	if err := packet.UnmarshalBinary(pdu.TPDU); err != nil {
		return nil, false, err
	}
	s.submissions = append(s.submissions, packet)
	if len(s.submissions) == 2 && s.loseSecondReply {
		return nil, true, context.DeadlineExceeded
	}
	return []string{"+CMGS: 35"}, true, nil
}

func TestListSMS(t *testing.T) {
	packets, err := sms.Encode([]byte("蜂窝网络"), sms.AsDeliver, sms.From("+15550100"))
	if err != nil {
		t.Fatal(err)
	}
	packets[0].SCTS.Time = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	raw, err := packets[0].MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	body, err := (&pdumode.PDU{TPDU: raw}).MarshalHexString()
	if err != nil {
		t.Fatal(err)
	}
	wire := &smsTransport{responses: map[string][]string{
		"AT+CPMS?":  {`+CPMS: "ME",1,255,"ME",1,255,"ME",1,255`},
		"AT+CMGL=4": {"+CMGL: 1,0,,20", "+CMT: ,5", "0000", body, "+CMGL: 2,1,,1"},
	}}
	result, err := NewDevice(wire).ListSMS(context.Background())
	if err != nil || result.Storage != "ME" || len(result.Messages) != 2 {
		t.Fatalf("inbox=%+v, error=%v", result, err)
	}
	first := result.Messages[0]
	if first.Text != "蜂窝网络" || first.Number != "+15550100" || first.Status != "Unread" || first.Error != "" {
		t.Errorf("first message=%+v", first)
	}
	if result.Messages[1].Error == "" {
		t.Errorf("missing data was silently accepted: %+v", result.Messages[1])
	}
}

func TestSendMultipartSMS(t *testing.T) {
	text := strings.Repeat("蜂窝", 60)
	wire := &smsTransport{responses: smsReadyResponses()}
	result, err := NewDevice(wire).SendSMS(context.Background(), "+15550100", text)
	if err != nil || result.Parts != 2 || len(result.References) != 2 || result.Uncertain {
		t.Fatalf("result=%+v, error=%v", result, err)
	}
	got, err := sms.Decode(wire.submissions)
	if err != nil || string(got) != text || !sms.IsCompleteMessage(wire.submissions) {
		t.Fatalf("multipart decode: %q, %v", got, err)
	}
}

func TestSendStopsAfterUncertainPart(t *testing.T) {
	wire := &smsTransport{responses: smsReadyResponses(), loseSecondReply: true}
	result, err := NewDevice(wire).SendSMS(context.Background(), "15550100", strings.Repeat("中", 180))
	if !errors.Is(err, context.DeadlineExceeded) || result.Parts != 3 || len(result.References) != 1 || !result.Uncertain || len(wire.submissions) != 2 {
		t.Fatalf("result=%+v, attempts=%d, error=%v", result, len(wire.submissions), err)
	}
	if got := wire.submissions[0].DA.TypeOfNumber(); got != tpdu.TonUnknown {
		t.Errorf("local number type=%v", got)
	}
}

func smsReadyResponses() map[string][]string {
	return map[string][]string{
		"AT+CPIN?":      {"+CPIN: READY"},
		"AT+CREG?":      {"+CREG: 0,0"},
		"AT+CEREG?":     {"+CEREG: 0,0"},
		"AT+C5GREG?":    {"+C5GREG: 0,1"},
		"AT+CGSMS?":     {"+CGSMS: 2"},
		"AT+CSCA?":      {`+CSCA: "+15550000",145`},
		`AT+QCFG="ims"`: {`+QCFG: "ims",1,0`},
	}
}

func TestSMSPreflight(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		response  string
		wantIssue string
	}{
		{"SIM locked", "AT+CPIN?", "+CPIN: SIM PIN", "SIM is not ready"},
		{"CS only on 5G", "AT+CGSMS?", "+CGSMS: 1", "circuit-switched only"},
		{"SMSC missing", "AT+CSCA?", `+CSCA: "",145`, "No SMS service centre"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire := &smsTransport{responses: smsReadyResponses()}
			wire.responses[test.command] = []string{test.response}
			device := NewDevice(wire)
			check, err := device.CheckSMS(context.Background())
			if err != nil || check.Ready || !strings.Contains(strings.Join(check.Issues, " "), test.wantIssue) {
				t.Fatalf("check=%+v, err=%v", check, err)
			}
			_, err = device.SendSMS(context.Background(), "15550100", "test")
			var failure *SMSFailure
			if !errors.As(err, &failure) || len(wire.submissions) != 0 {
				t.Fatalf("preflight allowed submission: err=%v, submissions=%d", err, len(wire.submissions))
			}
		})
	}
}

func TestSMS350DescriptionPreservesRejection(t *testing.T) {
	cause := at.Rejection("+CMS ERROR: 350")
	err := smsFailure(cause)
	var failure *SMSFailure
	var rejection at.Rejection
	if !errors.As(err, &failure) || !errors.As(err, &rejection) || !strings.Contains(err.Error(), "IMS") {
		t.Fatalf("error=%v", err)
	}
}

func TestListSMSSelectsIncomingStore(t *testing.T) {
	wire := &smsTransport{responses: map[string][]string{
		"AT+CPMS?":     {`+CPMS: "ME",3,255,"ME",3,255,"SM",4,50`},
		`AT+CPMS="SM"`: {"+CPMS: 4,50,3,255,4,50"},
	}}
	inbox, err := NewDevice(wire).ListSMS(context.Background())
	if err != nil || inbox.Storage != "SM" || strings.Join(wire.commands, "|") != `AT+CPMS?|AT+CPMS="SM"|AT+CMGF=0|AT+CMGL=4` {
		t.Fatalf("inbox=%+v, err=%v", inbox, err)
	}
}

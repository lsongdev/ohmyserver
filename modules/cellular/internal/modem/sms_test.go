package modem

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/pdumode"
	"github.com/warthog618/sms/encoding/tpdu"
)

type smsTransport struct {
	responses       map[string][]string
	submissions     []*tpdu.TPDU
	loseSecondReply bool
}

func (s *smsTransport) Query(_ context.Context, command string, match func(string) bool, _ func(string)) ([]string, error) {
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
	wire := &smsTransport{}
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
	wire := &smsTransport{loseSecondReply: true}
	result, err := NewDevice(wire).SendSMS(context.Background(), "15550100", strings.Repeat("中", 180))
	if !errors.Is(err, context.DeadlineExceeded) || result.Parts != 3 || len(result.References) != 1 || !result.Uncertain || len(wire.submissions) != 2 {
		t.Fatalf("result=%+v, attempts=%d, error=%v", result, len(wire.submissions), err)
	}
	if got := wire.submissions[0].DA.TypeOfNumber(); got != tpdu.TonUnknown {
		t.Errorf("local number type=%v", got)
	}
}

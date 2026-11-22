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

func inboundParts(t *testing.T, text, sender string, when time.Time, wide bool) []string {
	t.Helper()
	packets, err := sms.Encode([]byte(text), sms.AsDeliver, sms.From(sender))
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, packet := range packets {
		packet.SCTS.Time = when
		if wide {
			for i, header := range packet.UDH {
				if header.ID == 0 {
					packet.UDH[i] = tpdu.InformationElement{ID: 8, Data: []byte{0, header.Data[0], header.Data[1], header.Data[2]}}
				}
			}
		}
		raw, err := packet.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		body, err := (&pdumode.PDU{TPDU: raw}).MarshalHexString()
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func record(t *testing.T, storage string, index int, pdu string) smsRecord {
	t.Helper()
	got := decodeRecords(storage, []string{fmt.Sprintf("+CMGL: %d,0,,1", index), pdu})
	if len(got) != 1 || got[0].err != "" {
		t.Fatalf("decode result=%+v", got)
	}
	return got[0]
}

func TestAssembleMessagesAcrossStores(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprint(wide), func(t *testing.T) {
			text := strings.Repeat("长短信🙂测试", 24)
			bodies := inboundParts(t, text, "+15550100", time.Now().Truncate(time.Second), wide)
			if len(bodies) < 2 {
				t.Fatal("expected multipart test data")
			}
			var records []smsRecord
			for part := len(bodies) - 1; part >= 0; part-- {
				storage := "SM"
				if part%2 == 0 {
					storage = "ME"
				}
				records = append(records, record(t, storage, part, bodies[part]))
			}
			records = append(records, record(t, "SM", 40, bodies[0]))
			messages := assembleMessages(records)
			if len(messages) != 1 {
				t.Fatalf("got %d messages", len(messages))
			}
			message := messages[0]
			if message.Text != text || message.Received != len(bodies) || len(message.Missing) != 0 ||
				message.Error != "" || len(message.Sources) != len(bodies)+1 {
				t.Fatalf("message=%+v", message)
			}
		})
	}
}

func TestAssembleIncompleteMessages(t *testing.T) {
	bodies := inboundParts(t, strings.Repeat("中", 180), "+15550100", time.Now(), false)
	messages := assembleMessages([]smsRecord{
		record(t, "SM", 0, bodies[0]), record(t, "ME", 0, bodies[2]),
	})
	if len(messages) != 1 || messages[0].Received != 2 || messages[0].Parts != 3 ||
		len(messages[0].Missing) != 1 || messages[0].Missing[0] != 2 ||
		!strings.Contains(messages[0].Text, "[Missing part 2]") {
		t.Fatalf("messages=%+v", messages)
	}
}

func TestAssemblySeparatesSendersReferencesAndReusedReferences(t *testing.T) {
	when := time.Now().Truncate(time.Second)
	var records []smsRecord
	// Fresh encoders reuse the same reference. Different senders or days
	// must produce separate messages.
	for group, args := range []struct {
		sender string
		when   time.Time
	}{
		{"+15550100", when}, {"+15550200", when}, {"+15550100", when.Add(24 * time.Hour)},
	} {
		bodies := inboundParts(t, strings.Repeat("中", 100), args.sender, args.when, false)
		for part, body := range bodies {
			records = append(records, record(t, "SM", group*10+part, body))
		}
	}
	if got := assembleMessages(records); len(got) != 3 {
		t.Fatalf("got %d messages, expected 3", len(got))
	}
}

func TestAssemblyKeepsAmbiguousPartsSeparate(t *testing.T) {
	when := time.Now()
	first := inboundParts(t, strings.Repeat("甲", 100), "+15550100", when, false)
	second := inboundParts(t, strings.Repeat("乙", 100), "+15550100", when, false)
	var records []smsRecord
	for i, body := range append(first, second...) {
		records = append(records, record(t, "SM", i, body))
	}
	messages := assembleMessages(records)
	if len(messages) != len(records) {
		t.Fatalf("ambiguous parts were merged: %d messages", len(messages))
	}
	for _, message := range messages {
		if message.Error == "" {
			t.Fatal("missing ambiguity warning")
		}
	}
}

func TestListSMSRestoresStoreOnReadFailure(t *testing.T) {
	cause := errors.New("serial disconnected")
	wire := &smsTransport{
		responses: map[string][]string{
			"AT+CPMS?":  {`+CPMS: "ME",0,255,"ME",0,255,"SM",0,50`},
			"AT+CPMS=?": {`+CPMS: ("SM","ME"),("ME"),("SM")`},
		},
		queryErrors: map[string]error{"AT+CMGL=4": cause},
	}
	_, err := NewDevice(wire).ListSMS(context.Background())
	if !errors.Is(err, cause) || wire.commands[len(wire.commands)-1] != `AT+CPMS="ME"` {
		t.Fatalf("error=%v, commands=%q", err, wire.commands)
	}
}

func TestListSMSReportsUnavailableStore(t *testing.T) {
	wire := &smsTransport{
		responses: map[string][]string{
			"AT+CPMS?":  {`+CPMS: "ME",0,255,"ME",0,255,"SM",0,50`},
			"AT+CPMS=?": {`+CPMS: ("SM","ME"),("ME"),("SM")`},
		},
		queryErrors: map[string]error{`AT+CPMS="SM"`: at.Rejection("+CMS ERROR: 310")},
	}
	inbox, err := NewDevice(wire).ListSMS(context.Background())
	if err != nil || len(inbox.Stores) != 2 || inbox.Stores[0].Error == "" || inbox.Stores[1].Error != "" {
		t.Fatalf("inbox=%+v, err=%v", inbox, err)
	}
}

func TestAssemblySeparatesReferenceWidths(t *testing.T) {
	when := time.Now()
	eight := inboundParts(t, strings.Repeat("甲", 100), "+15550100", when, false)
	sixteen := inboundParts(t, strings.Repeat("乙", 100), "+15550100", when, true)
	var records []smsRecord
	for i, body := range append(eight, sixteen...) {
		records = append(records, record(t, "ME", i, body))
	}
	if got := assembleMessages(records); len(got) != 2 {
		t.Fatalf("8-bit and 16-bit references were merged: %d messages", len(got))
	}
}

package modem

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/pdumode"
	"github.com/warthog618/sms/encoding/tpdu"
)

type Message struct {
	Index  int        `json:"index"`
	Status string     `json:"status"`
	Number string     `json:"number,omitempty"`
	Text   string     `json:"text,omitempty"`
	Time   *time.Time `json:"time,omitempty"`
	Part   int        `json:"part,omitempty"`
	Parts  int        `json:"parts,omitempty"`
	Error  string     `json:"error,omitempty"`
}

type Inbox struct {
	Storage  string    `json:"storage"`
	Messages []Message `json:"messages"`
}

type Submission struct {
	References []int `json:"references"`
	Parts      int   `json:"parts"`
	Uncertain  bool  `json:"uncertain,omitempty"`
}

var phone = regexp.MustCompile(`^\+?[0-9]{1,15}$`)

type SMSInputError struct {
	Field   string
	Message string
}

func (e SMSInputError) Error() string { return e.Message }

func ValidateSMS(number, text string) error {
	if !phone.MatchString(number) {
		return SMSInputError{"number", "Enter 1–15 digits, optionally prefixed with +"}
	}
	if strings.TrimSpace(text) == "" || !utf8.ValidString(text) || len(text) > 2048 {
		return SMSInputError{"text", "Enter a message of up to 2048 UTF-8 bytes"}
	}
	return nil
}

// ListSMS reads the modem's current message storage. On Quectel, CMGL
// marks previously unread messages as read.
func (d Device) ListSMS(ctx context.Context) (Inbox, error) {
	inbox := Inbox{Messages: []Message{}}
	storage, err := d.query(ctx, "AT+CPMS?", prefixMatch("+CPMS:"))
	if err != nil {
		return inbox, err
	}
	if len(storage) != 1 {
		return inbox, InvalidReply("AT+CPMS?")
	}
	values := fields(storage[0])
	if len(values) == 0 {
		return inbox, InvalidReply("AT+CPMS?")
	}
	inbox.Storage = values[0]
	if _, err := d.query(ctx, "AT+CMGF=0", func(string) bool { return false }); err != nil {
		return inbox, err
	}
	lines, err := d.query(ctx, "AT+CMGL=4", listReplyMatcher())
	if err != nil {
		return inbox, err
	}
	inbox.Messages = decodeMessages(lines)
	return inbox, nil
}

func listReplyMatcher() func(string) bool {
	var expecting, unsolicited bool
	return func(line string) bool {
		switch {
		case strings.HasPrefix(line, "+CMGL:"):
			expecting, unsolicited = true, false
			return true
		case strings.HasPrefix(line, "+CMT:"), strings.HasPrefix(line, "+CDS:"), strings.HasPrefix(line, "+CBM:"):
			unsolicited = true
			return false
		}
		if _, err := hex.DecodeString(line); err != nil {
			return false
		}
		if unsolicited {
			unsolicited = false
			return false
		}
		if expecting {
			expecting = false
			return true
		}
		return false
	}
}

func decodeMessages(lines []string) []Message {
	messages := make([]Message, 0, len(lines)/2)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "+CMGL:") {
			continue
		}
		values := fields(line)
		index, status := number(values, 0), number(values, 1)
		if index == nil || status == nil {
			continue
		}
		message := Message{Index: *index, Status: "Unknown", Error: "Missing SMS data"}
		switch *status {
		case 0:
			message.Status = "Unread"
		case 1:
			message.Status = "Received"
		case 2:
			message.Status = "Draft"
		case 3:
			message.Status = "Sent"
		}
		if i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "+CMGL:") {
			i++
			decodeMessage(&message, lines[i], *status < 2)
		}
		messages = append(messages, message)
	}
	return messages
}

func decodeMessage(message *Message, raw string, inbound bool) {
	pdu, err := pdumode.UnmarshalHexString(raw)
	if err != nil {
		message.Error = "Invalid SMS data"
		return
	}
	packet := tpdu.TPDU{Direction: tpdu.MO}
	if inbound {
		packet.Direction = tpdu.MT
	}
	if err := packet.UnmarshalBinary(pdu.TPDU); err != nil {
		message.Error = "Invalid SMS data"
		return
	}
	switch packet.SmsType() {
	case tpdu.SmsDeliver:
		message.Number = address(packet.OA)
		message.Time = &packet.SCTS.Time
	case tpdu.SmsSubmit:
		message.Number = address(packet.DA)
	default:
		message.Error = "Unsupported SMS record type"
		return
	}
	text, err := sms.Decode([]*tpdu.TPDU{&packet})
	if err != nil {
		message.Error = "Unsupported SMS encoding"
		return
	}
	message.Text = string(text)
	message.Error = ""
	if parts, part, _, ok := packet.ConcatInfo(); ok {
		message.Parts, message.Part = parts, part
	}
}

func address(a tpdu.Address) string {
	if a.TypeOfNumber() == tpdu.TonInternational {
		return "+" + a.Addr
	}
	return a.Addr
}

type pduPart struct {
	length int
	hex    string
}

func encodeSMS(encoder *sms.Encoder, number, text string) ([]pduPart, error) {
	if err := ValidateSMS(number, text); err != nil {
		return nil, err
	}
	destination := tpdu.NewAddress(tpdu.FromNumber(number))
	if !strings.HasPrefix(number, "+") {
		destination.SetTypeOfNumber(tpdu.TonUnknown)
	}
	packets, err := encoder.Encode([]byte(text), sms.WithTemplateOption(tpdu.WithDA(destination)))
	if err != nil {
		return nil, SMSInputError{"text", "Cannot encode this message"}
	}
	if len(packets) > 6 {
		return nil, SMSInputError{"text", "Message exceeds six SMS parts"}
	}
	parts := make([]pduPart, 0, len(packets))
	for _, packet := range packets {
		raw, err := packet.MarshalBinary()
		if err != nil {
			return nil, err
		}
		pdu := pdumode.PDU{TPDU: raw}
		body, err := pdu.MarshalHexString()
		if err != nil {
			return nil, err
		}
		parts = append(parts, pduPart{length: len(raw), hex: body})
	}
	return parts, nil
}

// SendSMS encodes and submits a message in order. References identify parts
// accepted by the modem and are available even if a later part fails.
func (d Device) SendSMS(ctx context.Context, recipient, text string) (Submission, error) {
	parts, err := encodeSMS(d.encoder, recipient, text)
	if err != nil {
		return Submission{}, err
	}
	result := Submission{References: []int{}, Parts: len(parts)}
	if _, err := d.query(ctx, "AT+CMGF=0", func(string) bool { return false }); err != nil {
		return result, err
	}
	for _, part := range parts {
		partCtx, cancel := context.WithTimeout(ctx, 125*time.Second)
		lines, submitted, err := d.session.Submit(partCtx, fmt.Sprintf("AT+CMGS=%d", part.length), part.hex)
		cancel()
		if err != nil && len(lines) == 0 {
			result.Uncertain = submitted && !replyError(err)
			return result, err
		}
		if len(lines) > 0 {
			ref := number(fields(lines[0]), 0)
			if ref != nil {
				result.References = append(result.References, *ref)
				if err == nil {
					continue
				}
				return result, err
			}
		}
		result.Uncertain = submitted
		return result, errors.New("No valid SMS confirmation from the modem")
	}
	return result, nil
}

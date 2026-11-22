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

// MessageSource identifies a physical record, including a part of a long SMS.
type MessageSource struct {
	Storage string `json:"storage"`
	Index   int    `json:"index"`
	Part    int    `json:"part,omitempty"`
}

type Message struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Number   string          `json:"number,omitempty"`
	Text     string          `json:"text,omitempty"`
	Time     *time.Time      `json:"time,omitempty"`
	Parts    int             `json:"parts"`
	Received int             `json:"received"`
	Missing  []int           `json:"missing,omitempty"`
	Sources  []MessageSource `json:"sources"`
	Error    string          `json:"error,omitempty"`
}

type MessageStore struct {
	Name    string `json:"name"`
	Records int    `json:"records"`
	Error   string `json:"error,omitempty"`
}

type Inbox struct {
	Stores   []MessageStore `json:"stores"`
	Messages []Message      `json:"messages"`
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

// ListSMS reads physical SIM and modem stores, then reassembles messages.
// CMGL marks unread messages as read. Only the CPMS read store is switched;
// the original read store is restored before this operation completes.
func (d Device) ListSMS(ctx context.Context) (inbox Inbox, err error) {
	inbox = Inbox{Stores: []MessageStore{}, Messages: []Message{}}
	value, err := d.value(ctx, "AT+CPMS?", "+CPMS:")
	if err != nil {
		return inbox, err
	}
	values := fields("+CPMS: " + value)
	if len(values) < 9 {
		return inbox, InvalidReply("AT+CPMS?")
	}
	original := values[0]
	if original != "SM" && original != "ME" && original != "MT" {
		return inbox, InvalidReply("AT+CPMS?")
	}
	stores, err := d.messageStores(ctx)
	if err != nil {
		return inbox, err
	}
	current := original
	defer func() {
		if current == original {
			return
		}
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_, restoreErr := d.query(restoreCtx, `AT+CPMS="`+original+`"`, prefixMatch("+CPMS:"))
		if restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore SMS read storage: %w", restoreErr))
		}
	}()
	if _, err := d.query(ctx, "AT+CMGF=0", func(string) bool { return false }); err != nil {
		return inbox, err
	}
	var records []smsRecord
	for _, store := range stores {
		summary := MessageStore{Name: store}
		if current != store {
			_, err = d.query(ctx, `AT+CPMS="`+store+`"`, prefixMatch("+CPMS:"))
			if err == nil {
				current = store
			}
		}
		var lines []string
		if err == nil {
			lines, err = d.query(ctx, "AT+CMGL=4", listReplyMatcher())
		}
		if err != nil {
			if !replyError(err) {
				return inbox, err
			}
			summary.Error = err.Error()
			err = nil
		} else {
			decoded := decodeRecords(store, lines)
			summary.Records = len(decoded)
			records = append(records, decoded...)
		}
		inbox.Stores = append(inbox.Stores, summary)
	}
	inbox.Messages = assembleMessages(records)
	return inbox, nil
}

func (d Device) messageStores(ctx context.Context) ([]string, error) {
	value, err := d.value(ctx, "AT+CPMS=?", "+CPMS:")
	if err != nil {
		return nil, err
	}
	_, options, ok := strings.Cut(value, "(")
	options, _, closed := strings.Cut(options, ")")
	if !ok || !closed {
		return nil, InvalidReply("AT+CPMS=?")
	}
	values := fields("+CPMS: " + options)
	var stores []string
	// MT may alias ME or combine SM and ME. Read physical stores once.
	for _, name := range []string{"SM", "ME"} {
		for _, option := range values {
			if option == name {
				stores = append(stores, name)
				break
			}
		}
	}
	if len(stores) == 0 {
		for _, option := range values {
			if option == "MT" {
				return []string{"MT"}, nil
			}
		}
		return nil, InvalidReply("AT+CPMS=?")
	}
	return stores, nil
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
	check, err := d.CheckSMS(ctx)
	if err != nil {
		return result, err
	}
	if !check.Ready {
		return result, &SMSFailure{Message: "SMS was not submitted. " + strings.Join(check.Issues, " ")}
	}
	if _, err := d.query(ctx, "AT+CMGF=0", func(string) bool { return false }); err != nil {
		return result, err
	}
	for _, part := range parts {
		partCtx, cancel := context.WithTimeout(ctx, 125*time.Second)
		lines, submitted, err := d.session.Submit(partCtx, fmt.Sprintf("AT+CMGS=%d", part.length), part.hex)
		cancel()
		if err != nil && len(lines) == 0 {
			result.Uncertain = submitted && !replyError(err)
			return result, smsFailure(err)
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

package modem

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/pdumode"
	"github.com/warthog618/sms/encoding/tpdu"
)

type smsRecord struct {
	source      MessageSource
	status      string
	packet      *tpdu.TPDU
	fingerprint string
	err         string
}

func decodeRecords(store string, lines []string) []smsRecord {
	var records []smsRecord
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "+CMGL:") {
			continue
		}
		values := fields(lines[i])
		index, status := number(values, 0), number(values, 1)
		if index == nil || status == nil {
			continue
		}
		record := smsRecord{source: MessageSource{Storage: store, Index: *index}, status: "Unknown", err: "Missing SMS data"}
		switch *status {
		case 0:
			record.status = "Unread"
		case 1:
			record.status = "Received"
		case 2:
			record.status = "Draft"
		case 3:
			record.status = "Sent"
		}
		if i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "+CMGL:") {
			i++
			record.decode(lines[i], *status < 2)
		}
		records = append(records, record)
	}
	return records
}

func (r *smsRecord) decode(raw string, inbound bool) {
	frame, err := pdumode.UnmarshalHexString(raw)
	if err != nil {
		r.err = "Invalid SMS data"
		return
	}
	packet := &tpdu.TPDU{Direction: tpdu.MO}
	if inbound {
		packet.Direction = tpdu.MT
	}
	if err := packet.UnmarshalBinary(frame.TPDU); err != nil {
		r.err = "Invalid SMS data"
		return
	}
	if packet.SmsType() != tpdu.SmsDeliver && packet.SmsType() != tpdu.SmsSubmit {
		r.err = "Unsupported SMS record type"
		return
	}
	r.fingerprint = fmt.Sprintf("%d:%x", packet.Direction, sha256.Sum256(frame.TPDU))
	r.packet, r.err = packet, ""
	if parts, part, _, ok := packet.ConcatInfo(); ok {
		r.source.Part = part
		if parts < 1 || part < 1 || part > parts {
			r.err = "Invalid SMS part numbering"
		}
	}
}

type messageGroup struct {
	records   []smsRecord
	parts     map[int]*tpdu.TPDU
	first     time.Time
	total     int
	ambiguous bool
}

func recordTime(record smsRecord) time.Time {
	if record.packet != nil && record.packet.SmsType() == tpdu.SmsDeliver {
		return record.packet.SCTS.Time
	}
	return time.Time{}
}

func multipartKey(packet *tpdu.TPDU) string {
	total, _, ref, _ := packet.ConcatInfo()
	var headers tpdu.UserDataHeader
	width := 8
	for _, ie := range packet.UDH {
		if ie.ID == 8 {
			width = 16
		}
		if ie.ID != 0 && ie.ID != 8 {
			headers = append(headers, ie)
		}
	}
	return fmt.Sprintf("%v:%v:%d:%d:%d:%d:%d:%d:%v", packet.OA, packet.DA,
		packet.SmsType(), packet.DCS, packet.PID, width, ref, total, headers)
}

func assembleMessages(records []smsRecord) []Message {
	sort.SliceStable(records, func(i, j int) bool { return recordTime(records[i]).Before(recordTime(records[j])) })
	var groups []*messageGroup
	candidates := make(map[string][]*messageGroup)
	duplicates := make(map[string]*messageGroup)
	for _, record := range records {
		if record.err != "" {
			groups = append(groups, &messageGroup{records: []smsRecord{record}, total: 1})
			continue
		}
		if group := duplicates[record.fingerprint]; group != nil {
			group.records = append(group.records, record)
			continue
		}
		total, part, _, multipart := record.packet.ConcatInfo()
		var group *messageGroup
		if multipart && total > 1 {
			key := multipartKey(record.packet)
			when := recordTime(record)
			for _, candidate := range candidates[key] {
				// References are reused. Separate messages more than ten minutes
				// apart; collisions inside that window remain explicitly separate.
				if when.Sub(candidate.first) <= 10*time.Minute {
					group = candidate
					break
				}
			}
			if group == nil {
				group = &messageGroup{parts: make(map[int]*tpdu.TPDU), first: when, total: total}
				candidates[key] = append(candidates[key], group)
				groups = append(groups, group)
			}
			if group.parts[part] != nil {
				group.ambiguous = true
			}
			group.parts[part] = record.packet
		} else {
			part, total = 1, 1
			group = &messageGroup{parts: map[int]*tpdu.TPDU{1: record.packet}, total: 1}
			groups = append(groups, group)
		}
		group.records = append(group.records, record)
		duplicates[record.fingerprint] = group
	}
	messages := make([]Message, 0, len(groups))
	for _, group := range groups {
		if group.ambiguous {
			// Conflicting copies of one sequence number cannot be assigned
			// safely to the same message. Preserve each physical record.
			for _, record := range group.records {
				part := record.source.Part
				fragment := messageGroup{records: []smsRecord{record}, parts: map[int]*tpdu.TPDU{part: record.packet}, total: group.total}
				message := fragment.message()
				message.Error = "Conflicting SMS parts; shown separately"
				messages = append(messages, message)
			}
			continue
		}
		messages = append(messages, group.message())
	}
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Time == nil {
			return messages[j].Time != nil
		}
		return messages[j].Time != nil && messages[i].Time.Before(*messages[j].Time)
	})
	return messages
}

func (g *messageGroup) message() Message {
	first := g.records[0]
	message := Message{Status: first.status, Parts: g.total, Received: len(g.parts), Sources: []MessageSource{}, Error: first.err}
	var ids []string
	for _, record := range g.records {
		message.Sources = append(message.Sources, record.source)
		ids = append(ids, fmt.Sprintf("%s:%d", record.source.Storage, record.source.Index))
		if record.status == "Unread" || (record.status == "Draft" && message.Status == "Sent") {
			message.Status = record.status
		}
	}
	sort.Strings(ids)
	message.ID = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(ids, "|"))))
	if first.packet == nil {
		return message
	}
	message.Number = address(first.packet.DA)
	if first.packet.SmsType() == tpdu.SmsDeliver {
		message.Number = address(first.packet.OA)
		when := recordTime(first)
		message.Time = &when
	}
	var run []*tpdu.TPDU
	var text strings.Builder
	flush := func() {
		if len(run) == 0 {
			return
		}
		decoded, err := sms.Decode(run)
		if err != nil {
			message.Error = "Some SMS text could not be decoded"
			text.WriteString("[Undecodable text]")
		} else {
			text.Write(decoded)
		}
		run = nil
	}
	for part := 1; part <= g.total; part++ {
		packet := g.parts[part]
		if packet == nil {
			flush()
			message.Missing = append(message.Missing, part)
			fmt.Fprintf(&text, "\n[Missing part %d]\n", part)
		} else {
			run = append(run, packet)
		}
	}
	flush()
	message.Text = text.String()
	return message
}

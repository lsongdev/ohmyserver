package modem

import (
	"context"
	"strings"
)

// IMEI, IMSI and ICCID are device and subscriber identifiers. They are
// queried explicitly and never included in the periodic status snapshot.
func (d Device) IMEI(ctx context.Context) (string, error) {
	return d.identifier(ctx, "AT+CGSN", "+CGSN:", 15, 15)
}

func (d Device) IMSI(ctx context.Context) (string, error) {
	return d.identifier(ctx, "AT+CIMI", "+CIMI:", 5, 15)
}

func (d Device) ICCID(ctx context.Context) (string, error) {
	return d.identifier(ctx, "AT+QCCID", "+QCCID:", 18, 22)
}

func (d Device) identifier(ctx context.Context, command, prefix string, min, max int) (string, error) {
	parse := func(line string) string {
		if strings.HasPrefix(line, prefix) {
			line = payload(line)
		}
		if len(line) < min || len(line) > max || strings.Trim(line, "0123456789") != "" {
			return ""
		}
		return line
	}
	lines, err := d.query(ctx, command, func(line string) bool { return parse(line) != "" })
	if err != nil {
		return "", err
	}
	if len(lines) != 1 {
		return "", InvalidReply(command)
	}
	return parse(lines[0]), nil
}

// SignalQuality reports the standard AT+CSQ values. A nil member means the
// modem reported that the corresponding measurement was unavailable.
type SignalQuality struct {
	RSSIdBm *int
	BERCode *int
}

func (d Device) SignalQuality(ctx context.Context) (SignalQuality, error) {
	const command = "AT+CSQ"
	lines, err := d.query(ctx, command, prefixMatch("+CSQ:"))
	if err != nil {
		return SignalQuality{}, err
	}
	if len(lines) != 1 {
		return SignalQuality{}, InvalidReply(command)
	}
	values := fields(lines[0])
	rssi, ber := number(values, 0), number(values, 1)
	if rssi == nil || ber == nil || !(*rssi >= 0 && *rssi <= 31 || *rssi == 99) ||
		!(*ber >= 0 && *ber <= 7 || *ber == 99) {
		return SignalQuality{}, InvalidReply(command)
	}
	var result SignalQuality
	if *rssi != 99 {
		dbm := -113 + 2**rssi
		result.RSSIdBm = &dbm
	}
	if *ber != 99 {
		result.BERCode = ber
	}
	return result, nil
}

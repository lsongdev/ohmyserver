// Package stun implements unauthenticated UDP STUN binding transactions.
// It has no server list, NAT classification, or application lifecycle policy.
package stun

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

const cookie uint32 = 0x2112a442

// Response contains addresses supplied by a STUN binding response.
type Response struct {
	Source netip.AddrPort `json:"source"`
	Mapped netip.AddrPort `json:"mapped"`
	Origin netip.AddrPort `json:"origin,omitzero"`
	Other  netip.AddrPort `json:"other,omitzero"`
}

// ServerError is a STUN error response, distinct from a network timeout.
type ServerError struct {
	Code   int
	Reason string
}

func (e *ServerError) Error() string { return fmt.Sprintf("STUN %d: %s", e.Code, e.Reason) }

// Request creates a binding request. Change flags 2 and 4 request an alternate
// response port and IP, respectively (RFC 5780).
func Request(change uint32) ([]byte, [12]byte, error) {
	var tx [12]byte
	if _, err := rand.Read(tx[:]); err != nil {
		return nil, tx, err
	}
	n := 20
	if change != 0 {
		n += 8
	}
	b := make([]byte, n)
	binary.BigEndian.PutUint16(b, 1)
	binary.BigEndian.PutUint16(b[2:], uint16(n-20))
	binary.BigEndian.PutUint32(b[4:], cookie)
	copy(b[8:], tx[:])
	if change != 0 {
		binary.BigEndian.PutUint16(b[20:], 3)
		binary.BigEndian.PutUint16(b[22:], 4)
		binary.BigEndian.PutUint32(b[24:], change)
	}
	return b, tx, nil
}

// Parse validates a matching binding response, including attribute bounds.
// A nonmatching transaction is ignored by Exchange before calling Parse.
func Parse(b []byte, tx [12]byte) (Response, error) {
	var r Response
	if len(b) < 20 || binary.BigEndian.Uint32(b[4:]) != cookie || string(b[8:20]) != string(tx[:]) {
		return r, errors.New("invalid STUN header")
	}
	typ := binary.BigEndian.Uint16(b)
	if typ != 0x101 && typ != 0x111 {
		return r, errors.New("unexpected STUN message type")
	}
	n := int(binary.BigEndian.Uint16(b[2:]))
	if n%4 != 0 || n+20 != len(b) {
		return r, errors.New("invalid STUN message length")
	}
	var serverErr *ServerError
	var plain netip.AddrPort
	for p := 20; p < len(b); {
		if p+4 > len(b) {
			return r, errors.New("truncated STUN attribute")
		}
		t := binary.BigEndian.Uint16(b[p:])
		l := int(binary.BigEndian.Uint16(b[p+2:]))
		end := p + 4 + l
		next := p + 4 + ((l + 3) &^ 3)
		if end > len(b) || next > len(b) {
			return r, errors.New("truncated STUN attribute value")
		}
		v := b[p+4 : end]
		p = next
		switch t {
		case 1, 0x20, 0x802b, 0x802c:
			a, err := address(v, t == 0x20, tx)
			if err != nil {
				return r, err
			}
			switch t {
			case 1:
				plain = a
			case 0x20:
				r.Mapped = a
			case 0x802b:
				r.Origin = a
			case 0x802c:
				r.Other = a
			}
		case 9:
			if len(v) < 4 {
				return r, errors.New("invalid STUN error attribute")
			}
			serverErr = &ServerError{Code: int(v[2]&7)*100 + int(v[3]), Reason: string(v[4:])}
		}
	}
	if typ == 0x111 {
		if serverErr != nil {
			return r, serverErr
		}
		return r, errors.New("STUN error without error code")
	}
	if !r.Mapped.IsValid() {
		r.Mapped = plain
	}
	if !r.Mapped.IsValid() {
		return r, errors.New("STUN response has no mapped address")
	}
	return r, nil
}

func address(v []byte, xor bool, tx [12]byte) (netip.AddrPort, error) {
	if len(v) < 4 || v[0] != 0 {
		return netip.AddrPort{}, errors.New("invalid STUN address")
	}
	n := 4
	if v[1] == 2 {
		n = 16
	} else if v[1] != 1 {
		return netip.AddrPort{}, errors.New("unknown STUN address family")
	}
	if len(v) != 4+n {
		return netip.AddrPort{}, errors.New("invalid STUN address length")
	}
	port := binary.BigEndian.Uint16(v[2:])
	raw := append([]byte(nil), v[4:]...)
	if xor {
		var key [16]byte
		binary.BigEndian.PutUint32(key[:], cookie)
		copy(key[4:], tx[:])
		port ^= uint16(cookie >> 16)
		for i := range raw {
			raw[i] ^= key[i]
		}
	}
	ip, ok := netip.AddrFromSlice(raw)
	if !ok || port == 0 {
		return netip.AddrPort{}, errors.New("invalid STUN endpoint")
	}
	return netip.AddrPortFrom(ip.Unmap(), port), nil
}

// Exchange uses an existing, exclusively owned packet socket, preserving its
// NAT mapping. It retries once. expected is the required response source, which
// may differ from target for CHANGE-REQUEST. Cancellation interrupts reads.
func Exchange(ctx context.Context, conn net.PacketConn, target, expected netip.AddrPort, change uint32, timeout time.Duration) (Response, error) {
	b, tx, err := Request(change)
	if err != nil {
		return Response{}, err
	}
	stop := context.AfterFunc(ctx, func() { conn.SetReadDeadline(time.Now()) })
	defer stop()
	buf := make([]byte, 65535)
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return Response{}, err
		}
		deadline := time.Now().Add(timeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return Response{}, err
		}
		if _, err := conn.WriteTo(b, net.UDPAddrFromAddrPort(target)); err != nil {
			return Response{}, err
		}
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				if ctx.Err() != nil {
					return Response{}, ctx.Err()
				}
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					break
				}
				return Response{}, err
			}
			src, ok := from.(*net.UDPAddr)
			if !ok || src.AddrPort() != expected {
				continue
			}
			data := buf[:n]
			if n < 20 || string(data[8:20]) != string(tx[:]) {
				continue
			}
			r, err := Parse(data, tx)
			r.Source = src.AddrPort()
			return r, err
		}
	}
	return Response{}, fmt.Errorf("%s: %w", expected, context.DeadlineExceeded)
}

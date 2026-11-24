package stun

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

// RFC 5769 section 2.2: IPv4 binding response test vector.
func TestRFC5769(t *testing.T) {
	b, err := hex.DecodeString("0101003c2112a442b7e7a701bc34d686fa87dfae8022000b7465737420766563746f7220002000080001a147e112a643000800142b91f599fd9e90c38c7489f92af9ba53f06be7d780280004c07d4c96")
	if err != nil {
		t.Fatal(err)
	}
	var tx [12]byte
	copy(tx[:], b[8:20])
	r, err := Parse(b, tx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Mapped.String() != "192.0.2.1:32853" {
		t.Fatalf("mapped = %s", r.Mapped)
	}
	for _, mutate := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:len(b)-1] },
		func(b []byte) []byte { b[23] = 255; return b },
		func(b []byte) []byte { b[8] ^= 1; return b },
		func(b []byte) []byte { b[4] = 0; return b },
	} {
		if _, err := Parse(mutate(append([]byte{}, b...)), tx); err == nil {
			t.Fatal("accepted malformed response")
		}
	}
}

func TestXORIPv6(t *testing.T) {
	_, tx, _ := Request(0)
	ip := netip.MustParseAddr("2001:db8::1234")
	port := uint16(50001)
	b := make([]byte, 44)
	binary.BigEndian.PutUint16(b, 0x101)
	binary.BigEndian.PutUint16(b[2:], 24)
	binary.BigEndian.PutUint32(b[4:], cookie)
	copy(b[8:], tx[:])
	binary.BigEndian.PutUint16(b[20:], 0x20)
	binary.BigEndian.PutUint16(b[22:], 20)
	b[25] = 2
	binary.BigEndian.PutUint16(b[26:], port^uint16(cookie>>16))
	var key [16]byte
	binary.BigEndian.PutUint32(key[:], cookie)
	copy(key[4:], tx[:])
	raw := ip.As16()
	for i := range raw {
		b[28+i] = raw[i] ^ key[i]
	}
	r, err := Parse(b, tx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Mapped != netip.AddrPortFrom(ip, port) {
		t.Fatal(r.Mapped)
	}
}

func bindingResponse(req []byte) []byte {
	b := append([]byte{}, req[:20]...)
	binary.BigEndian.PutUint16(b, 0x101)
	binary.BigEndian.PutUint16(b[2:], 12)
	return append(b, 0, 1, 0, 8, 0, 1, 0xc3, 0x50, 203, 0, 113, 5)
}

func TestExchangeRetransmitAndCancellation(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 256)
		for i := 0; i < 2; i++ {
			n, from, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
			if i == 1 {
				server.WriteTo(bindingResponse(buf[:n]), from)
			}
		}
	}()
	addr := server.LocalAddr().(*net.UDPAddr).AddrPort()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := Exchange(ctx, client, addr, addr, 0, 20*time.Millisecond)
	if err != nil || r.Mapped.String() != "203.0.113.5:50000" {
		t.Fatalf("%+v, %v", r, err)
	}
	<-done
	ctx, cancel = context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	_, err = Exchange(ctx, client, addr, addr, 0, 5*time.Second)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("cancel: %v after %s", err, time.Since(start))
	}
}

func TestExchangeRejectsWrongSourceAndTransaction(t *testing.T) {
	server, _ := net.ListenPacket("udp4", "127.0.0.1:0")
	defer server.Close()
	rogue, _ := net.ListenPacket("udp4", "127.0.0.1:0")
	defer rogue.Close()
	client, _ := net.ListenPacket("udp4", "127.0.0.1:0")
	defer client.Close()
	go func() {
		b := make([]byte, 256)
		for i := 0; i < 2; i++ {
			n, from, e := server.ReadFrom(b)
			if e != nil {
				return
			}
			reply := bindingResponse(b[:n])
			rogue.WriteTo(reply, from)
			reply[8] ^= 1
			server.WriteTo(reply, from)
		}
	}()
	addr := server.LocalAddr().(*net.UDPAddr).AddrPort()
	_, err := Exchange(context.Background(), client, addr, addr, 0, 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("accepted invalid reply: %v", err)
	}
}

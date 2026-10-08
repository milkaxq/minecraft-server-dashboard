package minecraft

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"testing"
)

func TestStatusProtocol(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		size, err := readVarint(conn)
		if err != nil {
			done <- err
			return
		}
		body := make([]byte, size)
		if _, err = io.ReadFull(conn, body); err != nil {
			done <- err
			return
		}
		if len(body) < 8 || body[0] != 0 || body[len(body)-1] != 1 {
			done <- ErrProtocol
			return
		}
		size, err = readVarint(conn)
		if err != nil || size != 1 {
			done <- ErrProtocol
			return
		}
		var id [1]byte
		if _, err = io.ReadFull(conn, id[:]); err != nil || id[0] != 0 {
			done <- ErrProtocol
			return
		}
		payload, _ := json.Marshal(map[string]any{"players": map[string]int{"online": 7, "max": 30}})
		reply := append([]byte{0}, varint(int32(len(payload)))...)
		reply = append(reply, payload...)
		_, err = conn.Write(packet(reply))
		done <- err
	}()
	host, portRaw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portRaw)
	players, err := (Probe{}).Players(context.Background(), host, port)
	if err != nil || players.Online != 7 || players.Max != 30 {
		t.Fatal("status probe failed", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
func TestVarintAndOversizedResponses(t *testing.T) {
	for _, n := range []int32{0, 1, 127, 128, 65536, -1} {
		got, err := readVarint(bytes.NewReader(varint(n)))
		if err != nil || got != n {
			t.Fatal("invalid varint", n, got, err)
		}
	}
	if _, err := readVarint(bytes.NewReader([]byte{255, 255, 255, 255, 255, 1})); err == nil {
		t.Fatal("oversized varint accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write(varint(65537))
	}()
	host, raw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(raw)
	if _, err = (Probe{}).Players(context.Background(), host, port); err == nil {
		t.Fatal("unbounded response accepted")
	}
}

// Package minecraft implements only the read-only Java server-list status protocol.
// It cannot start processes, access files, or send Minecraft console commands.
package minecraft

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

var ErrProtocol = errors.New("minecraft status unavailable")

type Players struct {
	Online int
	Max    int
}
type Probe struct{}

func varint(n int32) []byte {
	v := uint32(n)
	b := []byte{}
	for {
		c := byte(v & 127)
		v >>= 7
		if v != 0 {
			c |= 128
		}
		b = append(b, c)
		if v == 0 {
			return b
		}
	}
}
func readVarint(r io.Reader) (int32, error) {
	var v uint32
	for i := 0; i < 5; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		if i == 4 && b[0]&240 != 0 {
			return 0, ErrProtocol
		}
		v |= uint32(b[0]&127) << uint(7*i)
		if b[0]&128 == 0 {
			return int32(v), nil
		}
	}
	return 0, ErrProtocol
}
func packet(body []byte) []byte { return append(varint(int32(len(body))), body...) }
func (Probe) Players(ctx context.Context, host string, port int) (Players, error) {
	if host == "" || len(host) > 253 || port < 1 || port > 65535 {
		return Players{}, ErrProtocol
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return Players{}, ErrProtocol
	}
	defer conn.Close()
	if d, ok := ctx.Deadline(); ok {
		if err = conn.SetDeadline(d); err != nil {
			return Players{}, ErrProtocol
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	handshake := []byte{0}
	handshake = append(handshake, varint(-1)...)
	handshake = append(handshake, varint(int32(len(host)))...)
	handshake = append(handshake, []byte(host)...)
	handshake = binary.BigEndian.AppendUint16(handshake, uint16(port))
	handshake = append(handshake, 1)
	data := append(packet(handshake), 1, 0)
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil || n == 0 {
			return Players{}, ErrProtocol
		}
		data = data[n:]
	}
	size, err := readVarint(conn)
	if err != nil || size < 3 || size > 65536 {
		return Players{}, ErrProtocol
	}
	body := make([]byte, size)
	if _, err = io.ReadFull(conn, body); err != nil {
		return Players{}, ErrProtocol
	}
	r := bytes.NewReader(body)
	id, err := readVarint(r)
	if err != nil || id != 0 {
		return Players{}, ErrProtocol
	}
	length, err := readVarint(r)
	if err != nil || length <= 0 || int(length) != r.Len() {
		return Players{}, ErrProtocol
	}
	payload := make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return Players{}, ErrProtocol
	}
	var response struct {
		Players *struct {
			Online int `json:"online"`
			Max    int `json:"max"`
		} `json:"players"`
	}
	if json.Unmarshal(payload, &response) != nil || response.Players == nil || response.Players.Online < 0 || response.Players.Max < 0 || response.Players.Online > 1000000 || response.Players.Max > 1000000 {
		return Players{}, ErrProtocol
	}
	return Players{Online: response.Players.Online, Max: response.Players.Max}, nil
}

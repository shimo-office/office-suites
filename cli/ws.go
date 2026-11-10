package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

type WSConn struct {
	conn net.Conn
	rd   *bufio.Reader
}

func dialWS(rawURL string) (*WSConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
	}
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 15*time.Second)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s:%s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, host, port, key)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	rd := bufio.NewReader(conn)
	resp, err := readHTTPHead(rd)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !strings.Contains(strings.SplitN(resp, "\r\n", 2)[0], " 101 ") {
		_ = conn.Close()
		return nil, fmt.Errorf("WebSocket 握手失败：%s", strings.SplitN(resp, "\r\n", 2)[0])
	}
	return &WSConn{conn: conn, rd: rd}, nil
}

func readHTTPHead(rd *bufio.Reader) (string, error) {
	var b strings.Builder
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return "", err
		}
		b.WriteString(line)
		if line == "\r\n" {
			return b.String(), nil
		}
	}
}

func (w *WSConn) Close() { _ = w.conn.Close() }

func (w *WSConn) Call(id int, method string, params any, timeout time.Duration) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	if err := w.sendJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	_ = w.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		msg, err := w.readMessage()
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		if int(num(m, "id")) != id {
			continue
		}
		if e, ok := m["error"].(map[string]any); ok {
			return nil, errors.New(str(e, "message"))
		}
		res, _ := m["result"].(map[string]any)
		return res, nil
	}
}

func (w *WSConn) sendJSON(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.writeFrame(0x1, payload)
}

func (w *WSConn) writeFrame(opcode byte, payload []byte) error {
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	header := []byte{0x80 | opcode}
	n := len(payload)
	switch {
	case n < 126:
		header = append(header, byte(0x80|n))
	case n < 65536:
		header = append(header, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(n))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(n))
	}
	masked := make([]byte, n)
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	_, err := w.conn.Write(append(append(header, mask...), masked...))
	return err
}

func (w *WSConn) readMessage() ([]byte, error) {
	var data []byte
	for {
		head := make([]byte, 2)
		if _, err := io.ReadFull(w.rd, head); err != nil {
			return nil, err
		}
		fin := head[0]&0x80 != 0
		opcode := head[0] & 0x0f
		masked := head[1]&0x80 != 0
		n := uint64(head[1] & 0x7f)
		if n == 126 {
			b := make([]byte, 2)
			if _, err := io.ReadFull(w.rd, b); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b))
		} else if n == 127 {
			b := make([]byte, 8)
			if _, err := io.ReadFull(w.rd, b); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b)
		}
		var mask []byte
		if masked {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(w.rd, mask); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(w.rd, payload); err != nil {
			return nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch opcode {
		case 0x9:
			_ = w.writeFrame(0xA, payload)
			continue
		case 0x8:
			return nil, errors.New("WebSocket 关闭")
		case 0x1, 0x2:
			data = payload
			if fin {
				return data, nil
			}
		case 0x0:
			data = append(data, payload...)
			if fin {
				return data, nil
			}
		}
	}
}

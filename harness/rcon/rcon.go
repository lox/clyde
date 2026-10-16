// Package rcon is a minimal Source RCON client for the Minecraft server's
// remote console.
package rcon

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	typeResponse = 0
	typeCommand  = 2
	typeAuth     = 3

	maxPayload = 4096 + 10
	ioTimeout  = 10 * time.Second
)

// ErrAuth means the server rejected the password.
var ErrAuth = errors.New("rcon: authentication failed")

// Client is one authenticated RCON connection. It is safe for concurrent use;
// commands are serialized.
type Client struct {
	mu   sync.Mutex
	conn net.Conn
	id   int32
}

// Dial connects to addr and authenticates with password.
func Dial(ctx context.Context, addr, password string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn}
	id, _, err := c.roundTrip(typeAuth, password)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if id == -1 {
		conn.Close()
		return nil, ErrAuth
	}
	return c, nil
}

// Exec runs one console command (without a leading slash) and returns the
// server's reply text.
func (c *Client) Exec(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, body, err := c.roundTrip(typeCommand, command)
	return body, err
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) roundTrip(typ int32, body string) (int32, string, error) {
	c.id++
	if err := c.conn.SetDeadline(time.Now().Add(ioTimeout)); err != nil {
		return 0, "", err
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, int32(len(body)+10))
	binary.Write(&buf, binary.LittleEndian, c.id)
	binary.Write(&buf, binary.LittleEndian, typ)
	buf.WriteString(body)
	buf.Write([]byte{0, 0})
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		return 0, "", fmt.Errorf("rcon: write: %w", err)
	}
	for {
		id, rtyp, rbody, err := c.read()
		if err != nil {
			return 0, "", err
		}
		// Servers may send an empty response packet before the auth reply,
		// and a reply that arrived after an earlier timeout belongs to an
		// older request; skip both.
		if typ == typeAuth && rtyp == typeResponse || id != c.id && id != -1 {
			continue
		}
		return id, rbody, nil
	}
}

func (c *Client) read() (id, typ int32, body string, err error) {
	var size int32
	if err = binary.Read(c.conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", fmt.Errorf("rcon: read: %w", err)
	}
	if size < 10 || size > maxPayload {
		return 0, 0, "", fmt.Errorf("rcon: bad packet size %d", size)
	}
	p := make([]byte, size)
	if _, err = io.ReadFull(c.conn, p); err != nil {
		return 0, 0, "", fmt.Errorf("rcon: read: %w", err)
	}
	id = int32(binary.LittleEndian.Uint32(p[0:4]))
	typ = int32(binary.LittleEndian.Uint32(p[4:8]))
	return id, typ, string(bytes.TrimRight(p[8:], "\x00")), nil
}

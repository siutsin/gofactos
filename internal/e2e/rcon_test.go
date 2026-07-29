// This file provides the minimal Source RCON client required by Factorio E2E.
package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	rconResponse = 0
	rconCommand  = 2
	rconAuth     = 3
	rconMaxBody  = 1 << 20
)

type rconClient struct {
	conn net.Conn
	next int32
}

type rconPacket struct {
	id     int32
	typeID int32
	body   string
}

// dialRCON establishes and authenticates the control channel for a test server.
func dialRCON(
	ctx context.Context,
	address, password string,
) (*rconClient, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial RCON: %w", err)
	}
	client := &rconClient{conn: conn, next: 1}
	if err := client.authenticate(password); err != nil {
		closeErr := conn.Close()
		if closeErr != nil {
			return nil, errors.Join(
				err,
				fmt.Errorf("close RCON: %w", closeErr),
			)
		}
		return nil, err
	}
	return client, nil
}

// authenticate prevents commands from running on an unverified connection.
func (c *rconClient) authenticate(password string) error {
	id := c.nextID()
	packet := rconPacket{id: id, typeID: rconAuth, body: password}
	if err := c.write(packet); err != nil {
		return fmt.Errorf("send RCON authentication: %w", err)
	}
	for {
		packet, err := c.read()
		if err != nil {
			return fmt.Errorf("read RCON authentication: %w", err)
		}
		if packet.id == -1 {
			return fmt.Errorf("RCON authentication rejected")
		}
		if packet.id == id {
			return nil
		}
	}
}

// command correlates one Factorio command with its matching response.
func (c *rconClient) command(command string) (string, error) {
	id := c.nextID()
	if err := c.write(rconPacket{
		id: id, typeID: rconCommand, body: command,
	}); err != nil {
		return "", fmt.Errorf("send RCON command: %w", err)
	}
	for {
		packet, err := c.read()
		if err != nil {
			return "", fmt.Errorf("read RCON command: %w", err)
		}
		if packet.id == id && packet.typeID == rconResponse {
			return strings.TrimSpace(packet.body), nil
		}
	}
}

// nextID separates replies when the server emits unrelated packets.
func (c *rconClient) nextID() int32 {
	id := c.next
	c.next++
	return id
}

// write enforces packet bounds and Source RCON's wire representation.
func (c *rconClient) write(packet rconPacket) error {
	deadline := time.Now().Add(10 * time.Second)
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	body := []byte(packet.body)
	if len(body) > rconMaxBody {
		return fmt.Errorf("RCON body is too large: %d bytes", len(body))
	}
	size := 4 + 4 + len(body) + 2
	buffer := make([]byte, 4+size)
	// Source RCON uses signed 32-bit fields on a little-endian byte stream.
	binary.LittleEndian.PutUint32(
		buffer[0:4],
		uint32(size), //nolint:gosec // Bounded by rconMaxBody above.
	)
	binary.LittleEndian.PutUint32(
		buffer[4:8],
		uint32(packet.id), //nolint:gosec // Required two's-complement encoding.
	)
	binary.LittleEndian.PutUint32(
		buffer[8:12],
		uint32(packet.typeID), //nolint:gosec // Required wire encoding.
	)
	copy(buffer[12:], body)
	if _, err := io.Copy(c.conn, bytes.NewReader(buffer)); err != nil {
		return fmt.Errorf("write packet: %w", err)
	}
	return nil
}

// read rejects malformed or oversized packets before exposing their payload.
func (c *rconClient) read() (rconPacket, error) {
	deadline := time.Now().Add(10 * time.Second)
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return rconPacket{}, fmt.Errorf("set read deadline: %w", err)
	}
	var sizeBuffer [4]byte
	if _, err := io.ReadFull(c.conn, sizeBuffer[:]); err != nil {
		return rconPacket{}, fmt.Errorf("read packet size: %w", err)
	}
	wireSize := binary.LittleEndian.Uint32(sizeBuffer[:])
	if wireSize < 10 || wireSize > rconMaxBody+10 {
		return rconPacket{}, fmt.Errorf(
			"invalid RCON packet size %d",
			wireSize,
		)
	}
	size := int(wireSize)
	buffer := make([]byte, size)
	if _, err := io.ReadFull(c.conn, buffer); err != nil {
		return rconPacket{}, fmt.Errorf("read packet body: %w", err)
	}
	if buffer[size-2] != 0 || buffer[size-1] != 0 {
		return rconPacket{}, fmt.Errorf("RCON packet has no terminator")
	}
	return rconPacket{
		id: int32( //nolint:gosec // Source RCON IDs are signed on the wire.
			binary.LittleEndian.Uint32(buffer[0:4]),
		),
		typeID: int32( //nolint:gosec // Source RCON types are signed on the wire.
			binary.LittleEndian.Uint32(buffer[4:8]),
		),
		body: string(buffer[8 : size-2]),
	}, nil
}

// Package protocol holds the wire types shared between the agent-tunnel
// server and the agent-tunnel CLI. See ../../docs/protocol.md for the
// human description.
package protocol

import (
	"encoding/json"
	"fmt"
)

// SlotKind enumerates the slot variants the server can route to.
type SlotKind string

const (
	SlotKindHTTP SlotKind = "http"
	SlotKindTCP  SlotKind = "tcp"
)

// SlotSpec is what the CLI sends on register and what the server stores.
type SlotSpec struct {
	Name             string   `json:"name"`
	Kind             SlotKind `json:"kind"`
	UpstreamHint     string   `json:"upstreamHint,omitempty"`
	DesiredSubdomain string   `json:"desiredSubdomain,omitempty"`
	DesiredPort      int      `json:"desiredPort,omitempty"`
	Anonymous        bool     `json:"anonymous,omitempty"`
}

// SlotAck is what the server returns on register_ack for each slot.
type SlotAck struct {
	Name       string   `json:"name"`
	Kind       SlotKind `json:"kind"`
	URL        string   `json:"url,omitempty"`
	PublicPort int      `json:"publicPort,omitempty"`
}

// FrameType discriminates control-plane frames carried on stream 0.
type FrameType string

const (
	FrameRegister    FrameType = "register"
	FrameRegisterAck FrameType = "register_ack"
	FrameEvent       FrameType = "event"
	FrameError       FrameType = "error"
	FramePing        FrameType = "ping"
	FramePong        FrameType = "pong"
)

// RegisterFrame is sent by the CLI immediately after the yamux session opens.
type RegisterFrame struct {
	Type   FrameType  `json:"type"`
	Tunnel string     `json:"tunnel"`
	Owner  string     `json:"owner"`
	Slots  []SlotSpec `json:"slots"`
	Token  string     `json:"token,omitempty"`
}

// RegisterAckFrame is returned by the server in response to RegisterFrame.
type RegisterAckFrame struct {
	Type   FrameType `json:"type"`
	Tunnel string    `json:"tunnel"`
	Slots  []SlotAck `json:"slots"`
}

// ErrorFrame is sent by the server whenever a request fails after the
// session is established. CLI should log and, depending on Code, retry or
// exit.
type ErrorFrame struct {
	Type    FrameType `json:"type"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Slot    string    `json:"slot,omitempty"`
}

// PingFrame / PongFrame are application-layer keepalives (in addition to
// websocket ping frames at the carrier layer).
type PingFrame struct {
	Type FrameType `json:"type"`
	Nonce uint64   `json:"nonce"`
}

type PongFrame struct {
	Type  FrameType `json:"type"`
	Nonce uint64    `json:"nonce"`
}

// EnvelopeType peeks the "type" field of any control frame without
// committing to a concrete struct shape.
type EnvelopeType struct {
	Type FrameType `json:"type"`
}

// DecodeEnvelope returns just the frame type — useful for routing.
func DecodeEnvelope(b []byte) (FrameType, error) {
	var e EnvelopeType
	if err := json.Unmarshal(b, &e); err != nil {
		return "", fmt.Errorf("decode envelope: %w", err)
	}
	if e.Type == "" {
		return "", fmt.Errorf("decode envelope: missing type")
	}
	return e.Type, nil
}

// StreamMeta is the JSON metadata header the server writes on every new
// proxied yamux stream before any user bytes flow. The CLI reads one line,
// then treats the stream as raw bytes.
type StreamMeta struct {
	Slot   string `json:"slot"`
	Remote string `json:"remote,omitempty"`
	TLS    bool   `json:"tls,omitempty"`
}

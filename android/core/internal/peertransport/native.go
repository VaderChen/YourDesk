package peertransport

import (
	"context"
	"net"
)

type Mode string

const Native Mode = ""

type Link interface {
	net.PacketConn
	Offer() string
}

func Open(context.Context, Mode, bool, string) (Link, error) { return nil, nil }
func Modes() []Mode                                          { return nil }
func Supports(m Mode) bool                                   { return m == Native }

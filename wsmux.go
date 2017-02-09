package wsmux

import (
	"context"
	"net"
)

type Mux struct {
}

func (m *Mux) Dial(network, address string) (net.Conn, error) {
	return nil, nil
}

func (m *Mux) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return nil, nil
}

func (m *Mux) Listen(network, address string) (net.Listener, error) {
	return &Listener{}, nil
}

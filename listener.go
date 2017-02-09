package wsmux

import "net"

type Listener struct {
}

func (l *Listener) Accept() (net.Conn, error) {
	return nil, nil
}

func (l *Listener) Close() error {
	return nil
}

func (l *Listener) Addr() net.Addr {
	return nil
}

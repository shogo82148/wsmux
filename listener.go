package wsmux

import (
	"errors"
	"net"
)

type Listener struct {
	mux     *Mux
	address string
	chConn  chan *Conn
	closed  chan struct{}
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.chConn:
		<-conn.accepted
		return conn, nil
	case <-l.closed:
		return nil, errors.New("listener closed")
	}
}

func (l *Listener) Close() error {
	l.mux.closeListener(l.address)
	return nil
}

func (l *Listener) Addr() net.Addr {
	return nil
}

func (l *Listener) receivedConn(conn *Conn) error {
	select {
	case l.chConn <- conn:
		return nil
	case <-l.closed:
		return errors.New("listener closed")
	}
}

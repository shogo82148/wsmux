package wsmux

import (
        "errors"
        "net"
        "time"
)

type Conn struct {
        id  uint64
        mux *Mux
}

func (c *Conn) Read(b []byte) (n int, err error) {
        return 0, nil
}

func (c *Conn) Write(b []byte) (n int, err error) {
        return 0, nil
}

func (c *Conn) Close() error {
        return nil
}

func (c *Conn) LocalAddr() net.Addr {
        return nil
}

func (c *Conn) RemoteAddr() net.Addr {
        return nil
}

func (c *Conn) SetDeadline(t time.Time) error {
        return &net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: errors.New("deadline not supported")}
}

func (c *Conn) SetReadDeadline(t time.Time) error {
        return &net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: errors.New("deadline not supported")}
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
        return &net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: errors.New("deadline not supported")}
}
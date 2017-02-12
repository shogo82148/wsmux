package wsmux

import (
	"errors"
	"io"
	"net"
	"time"
)

type Conn struct {
	id       uint64
	mux      *Mux
	accepted chan struct{}
	rejected chan struct{}
	closed   chan struct{}
	chReader chan io.Reader
	reader   io.Reader
}

func (c *Conn) Read(b []byte) (n int, err error) {
	for n == 0 && err == nil {
		r := c.reader
		if r == nil {
			select {
			case r = <-c.chReader:
				c.reader = r
			case <-c.closed:
				return 0, io.EOF
			}
		}
		n, err = r.Read(b)
		if err != nil {
			c.reader = nil
			c.mux.rdone <- struct{}{}
		}
		if err == io.EOF {
			// ignore EOF for next io.Reader
			err = nil
		}
	}
	return
}

func (c *Conn) Write(b []byte) (n int, err error) {
	return c.mux.writePacket(PacketData, c.id, b)
}

// Close sends a close request to the peer, and closes the connection.
func (c *Conn) Close() error {
	_, err := c.mux.writePacket(PacketClose, c.id, nil)
	if err != nil {
		return err
	}
	return c.close()
}

// close just closes the connection.
// Use this if the peer known that the connection is closing now.
func (c *Conn) close() error {
	c.mux.deleteConn(c.id)
	close(c.closed)
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

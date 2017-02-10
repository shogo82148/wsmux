package wsmux

import (
	"encoding/binary"
	"errors"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

type Conn struct {
	id       uint64
	mux      *Mux
	accepted chan struct{}
	rejected chan struct{}
}

func (c *Conn) sendDial() error {
	c.mux.wmu.Lock()
	defer c.mux.wmu.Unlock()

	w, err := c.mux.parent.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer w.Close()

	buf := make([]byte, 9)
	buf[0] = byte(PacketDial)
	binary.BigEndian.PutUint64(buf[1:], c.id)
	_, err = w.Write(buf)
	if err != nil {
		return err
	}
	return nil
}

func (c *Conn) sendAccept() error {
	c.mux.wmu.Lock()
	defer c.mux.wmu.Unlock()

	w, err := c.mux.parent.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer w.Close()

	buf := make([]byte, 9)
	buf[0] = byte(PacketAccept)
	binary.BigEndian.PutUint64(buf[1:], c.id)
	_, err = w.Write(buf)
	if err != nil {
		return err
	}
	return nil
}

func (c *Conn) sendReject() error {
	c.mux.wmu.Lock()
	defer c.mux.wmu.Unlock()

	w, err := c.mux.parent.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer w.Close()

	buf := make([]byte, 9)
	buf[0] = byte(PacketReject)
	binary.BigEndian.PutUint64(buf[1:], c.id)
	_, err = w.Write(buf)
	if err != nil {
		return err
	}
	return nil
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

package wsmux

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

type PacketType int8

const (
	PacketData PacketType = iota
	PacketDial
	PacketAccept
	PacketReject
	PacketClose
)

type Mux struct {
	wmu    sync.Mutex // Mutex for writing to the parent conn.
	parent *websocket.Conn

	mu     sync.RWMutex // Mutex for handling connection ids.
	prevID uint64
	conns  map[uint64]*Conn
}

func NewServer(parent *websocket.Conn) (*Mux, error) {
	m := &Mux{
		parent: parent,
		prevID: 0,
		conns:  make(map[uint64]*Conn, 16),
	}
	go m.readLoop()
	return m, nil
}

func NewClient(parent *websocket.Conn) (*Mux, error) {
	m := &Mux{
		parent: parent,
		prevID: 1,
		conns:  make(map[uint64]*Conn, 16),
	}
	go m.readLoop()
	return m, nil
}

func (m *Mux) Dial(network, address string) (net.Conn, error) {
	return m.DialContext(context.Background(), network, address)
}

func (m *Mux) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	id := atomic.AddUint64(&m.prevID, 2)
	conn := m.newConn(id)
	if err := conn.sendDial(); err != nil {
		return nil, err
	}

	select {
	case <-conn.accepted:
		return conn, nil
	case <-conn.rejected:
		m.deleteConn(conn.id)
		return nil, errors.New("wsmux: connection rejected")
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	}
}

func (m *Mux) newConn(id uint64) *Conn {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn := &Conn{
		id:       id,
		mux:      m,
		accepted: make(chan struct{}, 1),
		rejected: make(chan struct{}, 1),
	}
	m.conns[id] = conn
	return conn
}

func (m *Mux) getConn(id uint64) *Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conns[id]
}

func (m *Mux) deleteConn(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, id)
}

func (m *Mux) Listen(network, address string) (net.Listener, error) {
	return &Listener{}, nil
}

func (m *Mux) readLoop() {
	buf := make([]byte, 9)
	for {
		t, r, err := m.parent.NextReader()
		if err != nil {
			break
		}
		if t != websocket.BinaryMessage {
			continue
		}
		n, err := io.ReadFull(r, buf)
		if err != nil || n != 9 {
			break
		}
		packetType := PacketType(buf[0])
		connID := binary.BigEndian.Uint64(buf[1:])
		switch packetType {
		case PacketData:
			m.handleData(r, connID)
		case PacketDial:
			m.handleDial(r, connID)
		case PacketAccept:
			m.handleAccept(r, connID)
		case PacketReject:
			m.handleReject(r, connID)
		case PacketClose:
			m.handleClose(r, connID)
		}
	}
}

func (m *Mux) handleData(r io.Reader, connID uint64) {
}

func (m *Mux) handleDial(r io.Reader, connID uint64) {
	conn := m.newConn(connID)
	conn.sendAccept()

	// TODO: send the conn to a listener.
}

func (m *Mux) handleAccept(r io.Reader, connID uint64) {
	conn := m.getConn(connID)
	if conn == nil {
		return // ignore invalid packet
	}
	conn.accepted <- struct{}{}
}

func (m *Mux) handleReject(r io.Reader, connID uint64) {
	conn := m.getConn(connID)
	if conn == nil {
		return // ignore invalid packet
	}
	conn.rejected <- struct{}{}
}

func (m *Mux) handleClose(r io.Reader, connID uint64) {
}

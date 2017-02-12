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
	// Conn is the parent connection of the multiplexer.
	Conn *websocket.Conn
	wmu  sync.Mutex // Mutex for writing to the parent conn.

	mu     sync.RWMutex // Mutex for handling connection ids.
	prevID uint64
	conns  map[uint64]*Conn

	lmu       sync.RWMutex // Mutex for listeners
	listeners map[string]*Listener
	buf       [9]byte

	// rdone notifies child readers reach EOF
	rdone chan struct{}
}

func New(parent *websocket.Conn) *Mux {
	return &Mux{Conn: parent}
}

func (m *Mux) Server(parent *websocket.Conn) error {
	if parent == nil && m.Conn == nil {
		return errors.New("Mux.Conn is not set")
	}
	if parent != nil && m.Conn != nil {
		return errors.New("Mux.Conn is already set")
	}
	m.prevID = 0
	go m.readLoop()
	return nil
}

func (m *Mux) Client(parent *websocket.Conn) error {
	if parent == nil && m.Conn == nil {
		return errors.New("Mux.Conn is not set")
	}
	if parent != nil && m.Conn != nil {
		return errors.New("Mux.Conn is already set")
	}
	m.prevID = 1
	go m.readLoop()
	return nil
}

func (m *Mux) Dial(network, address string) (net.Conn, error) {
	return m.DialContext(context.Background(), network, address)
}

func (m *Mux) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	id := atomic.AddUint64(&m.prevID, 2)
	conn := m.newConn(id)
	_, err := m.writePacket(PacketDial, id, nil)
	if err != nil {
		return nil, err
	}

	select {
	case <-conn.accepted:
		return conn, nil
	case <-conn.rejected:
		m.closeConn(id)
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
		closed:   make(chan struct{}, 1),
		chReader: make(chan io.Reader, 1),
	}
	if m.conns == nil {
		m.conns = make(map[uint64]*Conn, 16)
	}
	m.conns[id] = conn
	return conn
}

func (m *Mux) getConn(id uint64) *Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conns[id]
}

func (m *Mux) Listen(network, address string) (net.Listener, error) {
	if network != "wsmux" {
		return nil, errors.New("unknown network")
	}
	l := &Listener{
		mux:     m,
		address: address,
		chConn:  make(chan *Conn),
		closed:  make(chan struct{}),
	}
	m.lmu.Lock()
	defer m.lmu.Unlock()
	if _, ok := m.listeners[address]; ok {
		return nil, errors.New("address is already used")
	}
	if m.listeners == nil {
		m.listeners = make(map[string]*Listener)
	}
	m.listeners[address] = l
	return l, nil
}

func (m *Mux) getListener(address string) *Listener {
	m.lmu.RLock()
	defer m.lmu.RUnlock()
	return m.listeners[address]
}

func (m *Mux) deleteListener(address string) {
	m.lmu.Lock()
	defer m.lmu.Unlock()
	delete(m.listeners, address)
}

func (m *Mux) readLoop() {
	buf := make([]byte, 9)
	if m.rdone == nil {
		m.rdone = make(chan struct{}, 1)
	}
	for {
		t, r, err := m.Conn.NextReader()
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
			m.closeConn(connID)
		}
	}
}

func (m *Mux) writePacket(packetType PacketType, connID uint64, b []byte) (int, error) {
	m.wmu.Lock()
	defer m.wmu.Unlock()

	buf := m.buf[:]
	buf[0] = byte(packetType)
	binary.BigEndian.PutUint64(buf[1:], connID)

	w, err := m.Conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}
	defer w.Close()
	_, err = w.Write(buf)
	if err != nil {
		return 0, err
	}
	if b == nil {
		return 0, nil
	}
	return w.Write(b)
}

func (m *Mux) handleData(r io.Reader, connID uint64) {
	conn := m.getConn(connID)
	if conn == nil {
		return // ignore invalid packet
	}
	conn.chReader <- r
	<-m.rdone
}

func (m *Mux) handleDial(r io.Reader, connID uint64) {
	l := m.getListener("address")
	if l == nil {
		m.writePacket(PacketReject, connID, nil)
		return
	}
	conn := m.newConn(connID)
	if err := l.receivedConn(conn); err != nil {
		m.writePacket(PacketReject, connID, nil)
		return
	}
	m.writePacket(PacketAccept, connID, nil)
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

func (m *Mux) closeConn(connID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.conns[connID]; ok {
		close(conn.closed)
		delete(m.conns, connID)
	}
}

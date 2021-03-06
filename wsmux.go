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

func (t PacketType) String() string {
	switch t {
	case PacketData:
		return "Data"
	case PacketDial:
		return "Dial"
	case PacketAccept:
		return "Accept"
	case PacketReject:
		return "Reject"
	case PacketClose:
		return "Close"
	}
	return ""
}

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
	if len(address) >= 256 {
		return nil, errors.New("wsmux: too long address")
	}
	buf := make([]byte, 0, len(address)+1)
	buf = append(buf, byte(len(address)))
	buf = append(buf, address...)
	_, err := m.writePacket(PacketDial, id, buf)
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

func (m *Mux) Listen(network, address string) (net.Listener, error) {
	if network != "wsmux" {
		return nil, errors.New("unknown network")
	}
	if len(address) >= 256 {
		return nil, errors.New("wsmux: too long address")
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

func (m *Mux) closeListener(address string) {
	m.lmu.Lock()
	defer m.lmu.Unlock()
	if l, ok := m.listeners[address]; ok {
		close(l.closed)
		delete(m.listeners, address)
	}
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
			err = m.handleData(r, connID)
		case PacketDial:
			err = m.handleDial(r, connID)
		case PacketAccept:
			err = m.handleAccept(r, connID)
		case PacketReject:
			err = m.handleReject(r, connID)
		case PacketClose:
			err = m.closeConn(connID)
		}
		if err != nil {
			break
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

func (m *Mux) handleData(r io.Reader, connID uint64) error {
	m.mu.RLock()
	conn, ok := m.conns[connID]
	if !ok {
		m.mu.RUnlock()
		return nil
	}
	select {
	case conn.chReader <- r:
		m.mu.RUnlock()
	default:
		m.mu.RUnlock()
		return errors.New("wsmux: unexpected parallel read")
	}
	<-m.rdone
	return nil
}

func (m *Mux) handleDial(r io.Reader, connID uint64) (err error) {
	// read the address
	var buf [255]byte
	_, err = io.ReadFull(r, buf[:1])
	if err != nil {
		return
	}
	len := int(buf[0])
	_, err = io.ReadFull(r, buf[:len])
	if err != nil {
		return
	}
	address := string(buf[:len])

	// find the listener
	m.lmu.RLock()
	defer m.lmu.RUnlock()
	l, ok := m.listeners[address]
	if !ok {
		// listener not found
		_, err = m.writePacket(PacketReject, connID, nil)
		return
	}

	// found!!
	_, err = m.writePacket(PacketAccept, connID, nil)
	if err != nil {
		return
	}
	conn := m.newConn(connID)
	l.chConn <- conn
	return
}

func (m *Mux) handleAccept(r io.Reader, connID uint64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, ok := m.conns[connID]
	if !ok {
		return nil
	}
	select {
	case conn.accepted <- struct{}{}:
	default:
		return errors.New("wsmux: duplicate accept packet")
	}
	return nil
}

func (m *Mux) handleReject(r io.Reader, connID uint64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, ok := m.conns[connID]
	if !ok {
		return nil
	}
	select {
	case conn.rejected <- struct{}{}:
	default:
		return errors.New("wsmux: duplicate accept packet")
	}
	return nil
}

func (m *Mux) closeConn(connID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.conns[connID]; ok {
		close(conn.closed)
		delete(m.conns, connID)
		select {
		case <-conn.chReader:
			m.rdone <- struct{}{}
		default:
		}
	}
	return nil
}

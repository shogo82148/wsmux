package wsmux

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func newTestMux() (*Mux, *Mux, func(), error) {
	// start a server for test
	type serverMux struct {
		mux  *Mux
		done chan<- struct{}
	}
	chanMux := make(chan serverMux, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		mux := New(conn)
		done := make(chan struct{}, 1)
		select {
		case chanMux <- serverMux{mux, done}:
			<-done
		default:
		}
	}))

	// dial to server
	u, err := url.Parse(srv.URL)
	if err != nil {
		return nil, nil, nil, err
	}
	u.Scheme = "ws"
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, nil, nil, err
	}
	mux := New(conn)
	smux := <-chanMux
	cleanup := func() {
		conn.Close()
		smux.done <- struct{}{}
	}
	return smux.mux, mux, cleanup, nil
}

func TestReject(t *testing.T) {
	smux, cmux, cleanup, err := newTestMux()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	smux.Server(nil)
	cmux.Client(nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = smux.DialContext(ctx, "network", "address")
	if err == nil {
		t.Error("want error, got not error")
	}
	select {
	case <-ctx.Done():
		t.Error("DialContext timeout")
	default:
	}
}

func TestMux(t *testing.T) {
	smux, cmux, cleanup, err := newTestMux()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	done := make(chan struct{}, 1)
	go func() {
		l, err := cmux.Listen("wsmux", "address")
		if err != nil {
			t.Fatal(err)
		}
		cconn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		var buf [5]byte
		n, err := io.ReadFull(cconn, buf[:])
		if err != nil {
			t.Error(err)
		}
		if n != 5 {
			t.Errorf("want 5, got %d", n)
		}
		if string(buf[:n]) != "Hello" {
			t.Errorf("want Hello, got %s", string(buf[:n]))
		}
		n, err = cconn.Read(buf[:])
		if err != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		if n != 0 {
			t.Errorf("want 0, got %d", n)
		}
		done <- struct{}{}
	}()
	smux.Server(nil)
	cmux.Client(nil)

	sconn, err := smux.Dial("network", "address")
	if err != nil {
		t.Fatal(err)
	}
	id := sconn.(*Conn).id
	n, err := sconn.Write([]byte("Hello"))
	if err != nil {
		t.Error(err)
	}
	if n != 5 {
		t.Errorf("want 5, got %d", n)
	}

	err = sconn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if smux.getConn(id) != nil {
		t.Error("close faild")
	}

	// wait for the peer
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("timeout")
	}
	if cmux.getConn(id) != nil {
		t.Error("close faild")
	}
}

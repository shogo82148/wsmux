package wsmux

import (
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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
		mux, err := NewServer(conn)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
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
	mux, err := NewClient(conn)
	if err != nil {
		return nil, nil, nil, err
	}

	smux := <-chanMux
	cleanup := func() {
		conn.Close()
		smux.done <- struct{}{}
	}
	return smux.mux, mux, cleanup, nil
}

func TestMux(t *testing.T) {
	smux, cmux, cleanup, err := newTestMux()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	sconn, err := smux.Dial("network", "address")
	if err != nil {
		t.Fatal(err)
	}
	if cmux.getConn(sconn.(*Conn).id) == nil {
		t.Error("dial faild")
	}
}

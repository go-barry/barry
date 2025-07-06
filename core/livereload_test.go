package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLiveReloader_ClientConnectsAndReceivesReload(t *testing.T) {
	lr := NewLiveReloader()

	server := httptest.NewServer(http.HandlerFunc(lr.Handler))
	defer server.Close()

	url := "ws" + server.URL[len("http"):]

	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	lr.BroadcastReload()

	ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read reload message: %v", err)
	}
	if string(msg) != "reload" {
		t.Errorf("expected 'reload' message, got %q", msg)
	}
}

func TestLiveReloader_RemovesDisconnectedClients(t *testing.T) {
	lr := NewLiveReloader()

	server := httptest.NewServer(http.HandlerFunc(lr.Handler))
	defer server.Close()

	url := "ws" + server.URL[len("http"):]
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect WebSocket: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_ = ws.Close()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastReload panicked after client disconnect: %v", r)
		}
	}()

	lr.BroadcastReload()
}

func TestLiveReloader_IgnoreUpgradeError(t *testing.T) {
	lr := NewLiveReloader()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	lr.Handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected HTTP 400 or 101 on upgrade failure, got %d", resp.StatusCode)
	}
}

func TestLiveReloader_BroadcastHandlesWriteFailure(t *testing.T) {
	lr := NewLiveReloader()

	server := httptest.NewServer(http.HandlerFunc(lr.Handler))
	defer server.Close()

	url := "ws" + server.URL[len("http"):]

	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect WebSocket: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_ = ws.Close()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastReload panicked after closed connection: %v", r)
		}
	}()

	lr.BroadcastReload()
}

func TestLiveReloader_BroadcastRemovesDeadConnection(t *testing.T) {
	lr := NewLiveReloader()

	server := httptest.NewServer(http.HandlerFunc(lr.Handler))
	defer server.Close()

	url := "ws" + server.URL[len("http"):]
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect WebSocket: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_ = ws.Close()

	time.Sleep(100 * time.Millisecond)

	lr.(*LiveReloader).lock.Lock()
	lr.(*LiveReloader).clients[ws] = true
	lr.(*LiveReloader).lock.Unlock()

	lr.BroadcastReload()

	lr.(*LiveReloader).lock.Lock()
	_, exists := lr.(*LiveReloader).clients[ws]
	lr.(*LiveReloader).lock.Unlock()

	if exists {
		t.Errorf("expected closed connection to be removed from clients map")
	}
}

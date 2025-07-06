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

	ws.Close()

	time.Sleep(100 * time.Millisecond)

	lr.lock.Lock()
	defer lr.lock.Unlock()

	if len(lr.clients) != 0 {
		t.Errorf("expected 0 clients after disconnect, found %d", len(lr.clients))
	}
}

func TestLiveReloader_IgnoreUpgradeError(t *testing.T) {
	lr := NewLiveReloader()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	lr.Handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 on failed upgrade, got %d", resp.StatusCode)
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

	time.Sleep(50 * time.Millisecond)

	lr.BroadcastReload()

	lr.lock.Lock()
	defer lr.lock.Unlock()

	if len(lr.clients) != 0 {
		t.Errorf("expected client to be removed after failed write, got %d remaining", len(lr.clients))
	}
}

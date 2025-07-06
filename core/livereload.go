package core

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type LiveReloaderInterface interface {
	BroadcastReload()
	Handler(http.ResponseWriter, *http.Request)
}

type LiveReloader struct {
	clients  map[*websocket.Conn]bool
	lock     sync.Mutex
	upgrader websocket.Upgrader
}

var NewLiveReloader = func() LiveReloaderInterface {
	return &LiveReloader{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (lr *LiveReloader) Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := lr.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	lr.lock.Lock()
	lr.clients[conn] = true
	lr.lock.Unlock()

	go func() {
		defer func() {
			lr.lock.Lock()
			delete(lr.clients, conn)
			lr.lock.Unlock()
			conn.Close()
		}()

		for {
			if _, _, err := conn.NextReader(); err != nil {
				break
			}
		}
	}()
}

func (lr *LiveReloader) BroadcastReload() {
	lr.lock.Lock()
	defer lr.lock.Unlock()

	for conn := range lr.clients {
		err := conn.WriteMessage(websocket.TextMessage, []byte("reload"))
		if err != nil {
			conn.Close()
			delete(lr.clients, conn)
		}
	}
}

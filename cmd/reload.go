package cmd

import (
	"log"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

var reloadC = make(chan struct{}, 1)

// wshandler handles the reloading when serve -H is used
func wshandler(ws *websocket.Conn) {
	pinger := time.NewTicker(time.Second * 30)
	defer func() {
		pinger.Stop()
		ws.Close()
	}()
	// only care about the one loaded message, it signals browser is loaded so we can unlock to build again.
	go func() {
		defer func() {
			// page is loaded so unlock and allow next build
			atomic.StoreUint32(&buildLock, 0)
		}()

		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			log.Println(err)
			return

		}
		return
	}()
	for {
		select {
		case <-reloadC:

			if err := websocket.Message.Send(ws, "reload"); err != nil {
				log.Println("websocket.Message.Send error", err)

			}
			// close as new connection each reload, otherwise broken pipe errors etc..
			return

		case <-pinger.C:
			ws.SetWriteDeadline(time.Now().Add(time.Second * 60))
			if err := websocket.Message.Send(ws, "ping"); err != nil {
				return
			}
		}
	}
}

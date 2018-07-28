package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/euforia/gossip/peers"
)

type httpServer struct {
	peers peers.Library
}

func (server *httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var data interface{}

	switch r.URL.Path {
	case "/peers":
		data = server.peers.List()

	case "/peers/closest":
		data = server.peers.Closest()

	case "/local":
		data = server.peers.Local()

	default:
		server.handleDistance(w, r)
		return

	}

	b, err := json.Marshal(data)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (server *httpServer) handleDistance(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/distance/") {
		w.WriteHeader(404)
		return
	}

	addr := strings.TrimPrefix(r.URL.Path, "/distance/")
	peers := server.peers.GetByAddress(addr)

	if peerCount(peers) == 0 {
		w.WriteHeader(404)
		return
	}

	local := server.peers.Local()
	duration := local.Coordinate.DistanceTo(peers[0].Coordinate)
	w.Write([]byte(duration.String()))
}

func peerCount(peers []*peers.Peer) (c int) {
	for i := range peers {
		if peers[i] != nil {
			c++
		}
	}
	return
}

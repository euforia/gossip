package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/euforia/gossip"
	"github.com/hexablock/iputil"
)

var (
	joinPeers = flag.String("peers", os.Getenv("GOSSIP_PEERS"), "Gossip peers")
	advAddr   = flag.String("addr", os.Getenv("GOSSIP_ADV_ADDR"), "Advertise address")
	debug     = flag.Bool("debug", false, "Debug mode")
)

func makeConfig() *gossip.Config {
	host, port, err := iputil.SplitHostPort(*advAddr)
	if err != nil {
		fmt.Println("Failed parse advertise address:", err)
		os.Exit(1)
	}

	conf := gossip.DefaultConfig()
	conf.BindAddr = host
	conf.BindPort = port
	conf.Name = *advAddr
	conf.Debug = *debug

	return conf
}

func main() {
	flag.Parse()

	conf := makeConfig()
	log := conf.Logger

	gsp, err := gossip.New(conf)
	if err != nil {
		log.Fatal(err)
	}

	// Register a new gossip pool.  You can register as many as 254
	pool := gsp.RegisterPool(gossip.DefaultLANPoolConfig(1))

	err = gsp.Start()
	if err != nil {
		log.Fatal(err)
	}

	// Join a pool
	if len(*joinPeers) > 0 {
		peers := strings.Split(*joinPeers, ",")
		_, err = pool.Join(peers)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Application http handler
	hs := &httpServer{peers: pool.Peers()}
	if err = http.Serve(gsp.Listener(), hs); err != nil {
		log.Fatal(err)
	}

}

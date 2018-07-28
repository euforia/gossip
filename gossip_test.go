package gossip

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"

	"github.com/stretchr/testify/assert"
)

func testMemlistConf() *memberlist.Config {
	mconf := memberlist.DefaultLocalConfig()

	mconf.GossipInterval = 50 * time.Millisecond
	mconf.ProbeInterval = 400 * time.Millisecond
	mconf.ProbeTimeout = 250 * time.Millisecond
	mconf.SuspicionMult = 1
	mconf.GossipNodes = 5

	return mconf
}

func testConf(host string, port int) *Config {
	conf := DefaultConfig()
	conf.Name = fmt.Sprintf("node%d", port)
	conf.BindAddr = host
	conf.BindPort = port
	return conf
}

func testPoolConfig(id int) *PoolConfig {
	conf := DefaultPoolConfig(id)
	conf.Memberlist = testMemlistConf()
	return conf
}

func makeNetwork(start, c int) ([]*Gossip, error) {
	out := make([]*Gossip, c)
	for i := 0; i < c; i++ {
		gossip, err := New(testConf("127.0.0.1", start+i))
		if err != nil {
			return nil, err
		}

		p1 := testPoolConfig(1)
		gossip.RegisterPool(p1)
		p2 := testPoolConfig(2)
		gossip.RegisterPool(p2)

		err = gossip.Start()
		if err != nil {
			return nil, err
		}
		out[i] = gossip
	}

	bs := fmt.Sprintf("127.0.0.1:%d", start)
	<-time.After(2000 * time.Millisecond)

	for i := range out[1:] {
		p := out[i].GetPool(1)
		_, err := p.Join([]string{bs})
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

func Test_Gossip(t *testing.T) {
	gnet, err := makeNetwork(54320, 5)
	if err != nil {
		t.Fatal(err)
	}
	<-time.After(1500 * time.Millisecond)

	poolMems := make([]*Pool, len(gnet))
	for i, g := range gnet {
		poolMems[i] = g.GetPool(1)
	}

	poolMems[1].Broadcast([]byte("from node 1"))
	<-time.After(1500 * time.Millisecond)

	tconn := gnet[0].TCPConnections()
	assert.NotNil(t, tconn)
	upkt := gnet[0].UDPPackets()
	assert.NotNil(t, upkt)

	local := poolMems[0].peers.Local()
	assert.NotNil(t, local.Coordinate)

	<-time.After(3500 * time.Millisecond)

	peers := poolMems[0].peers.List()
	for _, p := range peers[1:] {
		t.Log(peers[0].Coordinate.DistanceTo(p.Coordinate))
	}
}

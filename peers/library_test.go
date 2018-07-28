package peers

import (
	"net"
	"testing"

	"github.com/euforia/gossip/peers/peerspb"
	"github.com/stretchr/testify/assert"
)

func Test_InmemLibrary_Local(t *testing.T) {
	lib := NewInmemLibrary()
	local := &peerspb.Peer{Addr: net.ParseIP("127.0.0.1"), Port: 54321, PublicKey: []byte("first")}
	lib.SetLocal(local)
	gl := lib.Local()
	assert.Equal(t, local.Address(), gl.Address())

	p1 := &peerspb.Peer{Addr: net.ParseIP("127.0.0.1"), Port: 54320, PublicKey: []byte("foo")}
	p2 := &peerspb.Peer{Addr: net.ParseIP("127.0.0.1"), Port: 54319, PublicKey: []byte("bar")}
	i, err := lib.Add(p1, p2)
	assert.Nil(t, err)
	assert.Equal(t, 2, i)
	assert.Equal(t, 3, len(lib.List()))
	assert.Equal(t, 3, lib.Count())

	l := lib.GetByPublicKey([]byte("foo"))
	assert.Equal(t, 1, len(l))

	lib.Offline("127.0.0.1:54320")
	o := lib.GetByPublicKey([]byte("foo"))
	assert.True(t, o[0].Offline)

	p2 = &peerspb.Peer{Addr: net.ParseIP("127.0.0.1"), Port: 54318, PublicKey: []byte("bar")}
	lib.Update(p2)
	o2 := lib.GetByPublicKey([]byte("bar"))
	assert.Equal(t, p2.Address(), o2[0].Address())

	b, err := lib.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	lib2 := NewInmemLibrary()
	err = lib2.Restore(b)
	assert.Nil(t, err)

	assert.Equal(t, lib2.m[0].Address(), lib.m[0].Address())
	assert.Equal(t, 3, len(lib2.m))
	for i := range lib2.m {
		assert.Equal(t, "127.0.0.1", lib2.m[i].Addr.String())
	}

	lpeer := lib2.GetByPublicKey([]byte("first"))
	assert.Equal(t, 1, len(lpeer))
	assert.Equal(t, lib2.m[0].Address(), lpeer[0].Address())

	l2 := lib2.GetByAddress("127.0.0.1:54318")
	assert.Equal(t, 1, len(l2))
	assert.Equal(t, l2[0].Address(), "127.0.0.1:54318")

	n, err := lib2.Add(&peerspb.Peer{Addr: net.ParseIP("127.0.0.1"), Port: 54318})
	assert.NotNil(t, err)
	assert.Equal(t, 0, n)
}

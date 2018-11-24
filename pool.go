package gossip

import (
	"github.com/hashicorp/memberlist"
	"github.com/hexablock/log"
)

// Pool is a single gossip pool
type Pool struct {
	// pool id
	id int32

	// memberlist config
	conf *memberlist.Config
	// memberlist for pool
	*memberlist.Memberlist

	log *log.Logger
}

// NewPool returns a new gossip pool using the given config
func NewPool(conf *PoolConfig) *Pool {
	conf.Validate()
	p := &Pool{
		id:   conf.ID,
		conf: conf.Memberlist,
		log:  conf.Logger,
	}

	return p
}

// Start creates the underlying memberlist object
func (p *Pool) Start() (err error) {
	p.Memberlist, err = memberlist.Create(p.conf)
	if err == nil {
		p.log.Infof("Started pool=%d", p.id)
	}
	return
}

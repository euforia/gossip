package gossip

import (
	"io/ioutil"
	nlog "log"
	"os"

	"github.com/hashicorp/memberlist"
	"github.com/hexablock/log"
)

const (
	defaultBroadcastBuffSize int = 32
)

// Config holds the config instantiate a Gossip instance
type Config struct {
	Name          string
	AdvertiseAddr string
	AdvertisePort int
	BindAddr      string
	BindPort      int
	Logger        *log.Logger
	Debug         bool
}

// Validate validates thte config
func (conf *Config) Validate() {
	conf.Logger.EnableDebug(conf.Debug)
}

// DefaultConfig returns a default config to instantiate a Gossip instance
func DefaultConfig() *Config {
	return &Config{
		BindAddr: "0.0.0.0",
		Logger:   log.NewDefaultLogger(),
	}
}

// PoolConfig is the config for a single gossip pool
type PoolConfig struct {
	ID                int32
	BroadcastBuffSize int
	Delegate          Delegate
	Memberlist        *memberlist.Config
	Logger            *log.Logger
	Debug             bool
}

// Validate validates the pool config
func (conf *PoolConfig) Validate() {
	if conf.Debug {
		conf.Memberlist.Logger = nlog.New(os.Stderr, "", nlog.LstdFlags|nlog.Lmicroseconds)
	} else {
		conf.Memberlist.Logger = nlog.New(ioutil.Discard, "", nlog.LstdFlags)
	}
	if conf.BroadcastBuffSize <= 0 {
		conf.BroadcastBuffSize = defaultBroadcastBuffSize
	}
}

// DefaultPoolConfig returns a base config to init a new gossip pool
func DefaultPoolConfig(id int32) *PoolConfig {
	return &PoolConfig{
		ID:                id,
		BroadcastBuffSize: defaultBroadcastBuffSize,
	}
}

// DefaultWANPoolConfig returns a sane config suitable for WAN based pool
func DefaultWANPoolConfig(id int32) *PoolConfig {
	c := DefaultPoolConfig(id)
	c.Memberlist = memberlist.DefaultWANConfig()
	return c
}

// DefaultLANPoolConfig returns a sane config suitable for LAN based pool
func DefaultLANPoolConfig(id int32) *PoolConfig {
	c := DefaultPoolConfig(id)
	c.Memberlist = memberlist.DefaultLANConfig()
	return c
}

// DefaultLocalPoolConfig returns a sane config suitable for local pool
func DefaultLocalPoolConfig(id int32) *PoolConfig {
	c := DefaultPoolConfig(id)
	c.Memberlist = memberlist.DefaultLocalConfig()
	return c
}

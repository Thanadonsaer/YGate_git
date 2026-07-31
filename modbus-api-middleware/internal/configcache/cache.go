// Package configcache holds the middleware's active configuration in
// memory so request handling and Modbus polling never query SQLite
// per-request. SQLite remains the durable source of truth (loaded into the
// cache at startup and after every write); the cache is purely a read-path
// optimization plus the mechanism that makes a config push hot-swappable
// without a process restart.
package configcache

import (
	"sync/atomic"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

// Config is an immutable snapshot of the middleware's active configuration.
// Callers must never mutate a Config obtained from Load -- build a new one
// and Swap it in instead.
type Config struct {
	Version     int64
	Brands      []domain.Brand
	DeviceSets  map[int64]domain.DeviceSet
	Connections map[int64]domain.ConnectionConfig
	Plants      []domain.Plant
}

// Cache is a lock-free, read-mostly holder for the active Config. Reads
// (every poll tick, every local-UI GET) far outnumber writes (a central
// push or a local edit, at most a few times a day), and a pointer swap
// makes a reader observing a half-applied config impossible by
// construction -- unlike a mutex-guarded struct, which only prevents that
// if every call site remembers to lock around every field it touches.
type Cache struct {
	ptr atomic.Pointer[Config]
}

func New() *Cache {
	c := &Cache{}
	c.ptr.Store(&Config{DeviceSets: map[int64]domain.DeviceSet{}, Connections: map[int64]domain.ConnectionConfig{}})
	return c
}

// Load returns the current snapshot. Never nil.
func (c *Cache) Load() *Config {
	return c.ptr.Load()
}

// Swap atomically replaces the active snapshot.
func (c *Cache) Swap(cfg *Config) {
	c.ptr.Store(cfg)
}

// RebuildFromStore reads the full configuration back out of SQLite. Call
// this after any write to SQLite (a local edit or an applied central push)
// and Swap the result in so the cache and SQLite never silently diverge.
func RebuildFromStore(st *store.Store) (*Config, error) {
	brands, err := st.Brands()
	if err != nil {
		return nil, err
	}
	deviceSetList, err := st.DeviceSets()
	if err != nil {
		return nil, err
	}
	deviceSets := make(map[int64]domain.DeviceSet, len(deviceSetList))
	for _, ds := range deviceSetList {
		deviceSets[ds.DeviceSetID] = ds
	}
	connectionList, err := st.ConnectionsWithStatus()
	if err != nil {
		return nil, err
	}
	connections := make(map[int64]domain.ConnectionConfig, len(connectionList))
	for _, conn := range connectionList {
		connections[conn.ConnectionID] = conn
	}
	plants, err := st.Plants()
	if err != nil {
		return nil, err
	}
	version, err := st.CurrentConfigVersion()
	if err != nil {
		return nil, err
	}
	return &Config{Version: version, Brands: brands, DeviceSets: deviceSets, Connections: connections, Plants: plants}, nil
}

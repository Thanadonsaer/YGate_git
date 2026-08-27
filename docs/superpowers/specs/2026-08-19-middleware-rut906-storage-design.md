# Middleware RUT906 Storage Design

## Goal

Run CHPP Middleware on the RUT906's MIPS-based RutOS while keeping the existing SQLite storage unchanged for Windows and preserving the current middleware behavior and configuration model.

## Constraints

- Windows continues to use `middleware.db` through SQLite.
- RUT906 uses a pure-Go file backend because the current `modernc.org/libc` SQLite dependency cannot build for `linux/mips`.
- Existing web routes, domain types, configuration fields, outbox retry semantics, and callers keep their current behavior.
- The RUT906 backend must support concurrent poll, delivery, and web operations, atomic persistence, and recovery after an interrupted write.
- No dependency may require CGO or an external database service on RUT906.

## Design

Keep the public `store.Store` method surface used by `app`, `web`, `realtimeclient`, and `configcache`, but split the implementation by build tags. The SQLite implementation remains the default implementation for Windows and currently supported desktop/server targets. A MIPS implementation stores one versioned JSON document in a temporary file and atomically renames it into place under a process mutex; reads use an in-memory snapshot, and each mutation persists a complete snapshot.

The MIPS file backend uses a configurable path passed through the existing `-db` flag, with a default filename such as `middleware.store`. It stores gateway configuration, brands, device sets, addresses, plants, connections, profiles/devices, outbox events, delivery logs, poll logs, and config history in one document. IDs and timestamps remain the same logical values as the SQLite implementation. The update bridge is excluded from MIPS until it is ported to the same backend.

## Compatibility

The JSON/file format is versioned and has explicit migration defaults for newly added fields. It is not a transparent SQLite file conversion; configuration transfer remains through the existing settings export/import flow. Windows `middleware.db` files are not read by the MIPS backend.

## Verification

- Existing SQLite test suite remains green on the host platform.
- MIPS-target compilation proves no SQLite dependency is included in the MIPS build.
- File-backend tests cover empty initialization, config round-trip, concurrent mutations, outbox retry/delivery, atomic replacement, and malformed/older document recovery behavior.
- Manual smoke test covers web configuration, Modbus polling, queued delivery, restart persistence, and license activation on RUT906.

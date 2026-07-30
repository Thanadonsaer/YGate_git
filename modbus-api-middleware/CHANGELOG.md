# Changelog

## v0.2.6

- Restored the required Device Model selector in the add/edit Device dialog.
- Kept Device Model creation in Register Metadata instead of nesting it in the Device dialog.
- Added an embedded-page regression test for active dialog fields hidden by HTML comments.
- Preserved runtime `middleware.db` and `license.json` files during release builds.

## v0.2.5

- Opened an interactive Windows Service Manager when launching the EXE without arguments.
- Added menu actions for install/update, status, start, stop, restart and uninstall.
- Added menu actions for opening the Web UI, showing the machine ID and activating a license.
- Kept port 8081 closed until the user installs/starts the Service or explicitly uses `-run`.

## v0.2.4

- Added `-service install|status|start|stop|restart|uninstall` to the Middleware EXE.
- Prevented no-argument console launches from opening SQLite or port 8081.
- Required explicit `-run` or `-gui` for console web-server mode.
- Kept Windows Service startup unchanged after a completed installation.

## v0.2.3

- Removed Windows Service lifecycle controls and license activation from the web UI.
- Returned `404` for the retired `/service` and `/api/service/*` routes.
- Kept service installation, control, and license activation in the Terminal.

## v0.2.2

- Guarded optional UI elements before updating their HTML.
- Kept patch uploads in staged state when Middleware is not running as a service.
- Applied and restarted automatically only when service mode is available.

## v0.2.1

- Fixed Windows Service patch apply by waiting for the service to stop before replacing the binary.
- Made the web patch upload confirm once, then apply and restart automatically without a UAC prompt.
- Added an updater result file and automatic service restart attempt when an update fails.

## v0.2.0

- Replaced Middleware canonical `dataItemMap` output with raw `registerAddressMap`.
- Added schema version `2.0` delivery to `/api/v2/ingestion/register-readings`.
- Removed canonical-key and unit normalization from the polling path.
- Kept legacy Address Configuration columns only for SQLite/config import compatibility.

## v0.1.1

- Added web `Update Patch` menu for staged feature update ZIP upload.
- Added update manifest validation for app, version, OS/arch and SHA256.
- Added patch upload/apply/rollback endpoints; update actions are gated by service/systemd mode and license activation.
- Added update patch ZIP artifacts in `build\release` without `middleware.db`.
- Added Plant delete cascade for devices/connections, API log clear/export, and Live Monitor session log export from prior feature patch work.


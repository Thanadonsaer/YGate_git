# Register Metadata Form Simplification — Design

Status: Approved (design), pending implementation plan
Date: 2026-08-01

## Context

`apps/web`'s Register Metadata page (`apps/web/app/features/register-metadata/register-metadata-page.tsx`, `AddressMetadataDialog`) edits `DeviceModelRegisterMetadata` rows — the master catalog of register/telemetry-point definitions attached to a Device Model. Each row can serve two purposes at once: (1) a **display-only** telemetry point (used by v1 JSON ingestion, matched by `addressKey`, formatted per `dataType`/`unit`/`decimals`), and/or (2) a **Modbus-polled** register (decoded per `modbusFunctionCode`/`modbusRegister`/`modbusWordOrder`/`modbusDataType`), enforced today by backend validation as "all four Modbus fields together, or none" (`services/platform-api/internal/core/devices.go`, `validateRegisterMetadata`).

The user found the current flat form confusing: `addressKey` (a free-text key) sits next to `modbusRegister` (a numeric register address) with no visual relationship, and `dataType` (number/boolean/text/enum, for display formatting) sits next to `modbusDataType` (U16/I16/U32/I32/U64/FLOAT32, for wire decoding) — both pairs look redundant even though they serve different purposes. This spec simplifies the **form and list view only** — no backend schema, validation rule, or API contract changes.

## Decisions (confirmed with user during brainstorming)

- Scope covers the whole `AddressMetadataDialog` form, not just rows that already have Modbus fields set.
- No backend/schema changes: `addressKey` remains the row's real primary/lookup key (used by v1 ingestion field-name matching and as the API's identity for PUT/DELETE), `dataType` and `modbusDataType` remain separate columns. This is a frontend UX simplification, not a data-model merge (Approach B, a full key/column merge, was considered and rejected — too invasive for the backend's existing v1-ingestion key-matching behavior).
- The three tests that decide the approach are done via **conditional visibility and auto-fill**, not field removal.

## Form structure

Reorganize `AddressMetadataDialog`'s form into three visually grouped sections, in this order:

1. **Address / Key** — unchanged field, unchanged behavior (`readOnly` when editing an existing row, editable text input when creating).
2. **Modbus Register** — Function code, Register, Word order, Modbus type (the existing four Modbus inputs, grouped under one visual block instead of floating inline).
3. **Display** — Data type, Unit, Scale, Offset, Decimals (existing fields, grouped under one visual block).
4. **Notes** — unchanged, full-width, last.

## Behavior rules

### Auto-fill `Address / Key` from the Modbus register

When creating a new row (`item` is `null`) and the user has not yet manually typed into `Address / Key`, selecting both a Function code and a Register auto-fills `Address / Key` with `` `${functionCode}:${register}` `` (e.g. `3:40001`) — matching the existing fallback format the one-off seed script already uses for addresses with no canonical name. The user can still type over this value at any time; auto-fill only fires while the field is untouched (empty or still matching the last auto-fill it computed — a manual edit stops future auto-fills from overwriting it). This is a pure UX convenience: it does not change what gets submitted, only what's pre-populated.

Editing an existing row keeps `Address / Key` `readOnly`, exactly as today — auto-fill only applies during creation.

### Conditional `Data type` visibility

The `Data type` picker (number/boolean/text/enum) is hidden whenever `Modbus type` has a value set. While hidden, the field's underlying state is forced to `"number"` (matching what every Modbus-decoded value actually is) so submission never sends an empty/stale `dataType`. The picker reappears, showing whatever value was last set (defaulting to `"number"` if never touched), the moment `Modbus type` is cleared back to unset — covering the "user changes their mind mid-edit" case from both directions.

### List view — `Type` column

In the parent list (`filteredItems.map(...)` row rendering in `RegisterMetadataPage`), the `Type` column currently always renders `item.dataType`. Change it to render `item.modbusDataType` when present, falling back to `item.dataType` otherwise — so Modbus-backed rows show their actual wire type (`FLOAT32`, `U16`, ...) instead of the uniform `"number"` every Modbus row would otherwise display.

## Explicitly out of scope

- No new frontend validation for `Address / Key` collisions — the existing backend upsert-by-`addressKey` semantics (silent overwrite on duplicate key within the same model) are unchanged and not warned about in the UI.
- No changes to `DeviceModelDialog` (Brand/Type/Model/Source Type fields) — unrelated to this form.
- No changes to backend validation (`validateRegisterMetadata`), the database schema, the OpenAPI contract, or the one-off seed script.
- No changes to `RegisterMetadata` (the per-Device table, as opposed to `DeviceModelRegisterMetadata`, the per-Model template this page edits) — out of scope, not touched by this page today either.

## Verification

- `npx tsc --noEmit` clean.
- Manual: create a new Address with Function code + Register set → confirm `Address/Key` auto-fills as `fc:register` and the Data type picker is hidden; clear Function code → confirm Data type picker reappears with `"number"` pre-selected; type directly into `Address/Key` before setting Modbus fields → confirm auto-fill never overwrites the manual value afterward.
- Manual: edit an existing Modbus-backed row → confirm `Address/Key` stays read-only and Data type stays hidden (already `"number"` from its stored value).
- Manual: create a display-only row (no Modbus fields) → confirm `Address/Key` never auto-fills and the Data type picker is visible and required, exactly as today.
- Manual: list view → confirm Modbus rows show their `modbusDataType` (e.g. `FLOAT32`) in the `Type` column, and display-only rows still show `dataType`.

# Register Profiles, Metadata-Driven Alarms, and Login Status Design

Date: 2026-08-14

## Summary

This design adds three related capabilities:

1. Login responses that distinguish an unverified registration from a verified account that is still waiting for access, without exposing account state to callers who do not know the password.
2. Reusable Register Profiles that let multiple Device Models share register decoding, presentation metadata, exact value mappings, and bitmask mappings.
3. Metadata-driven inverter alarms that use those mappings to create normal Alarm Log events, realtime notifications, acknowledgements, and optional Plant-scoped email notifications.

The telemetry store remains raw-only. A central resolver applies the current Register Profile at read/evaluation time, preserving the existing numeric API while adding display values. Alarm events snapshot their interpreted meaning when they open so historical alarms do not change after a Profile edit.

## Goals

- Tell a user with a correct password whether email verification or Admin access approval is still required.
- Provide a safe resend-verification action from the Login screen.
- Define register metadata once and share it across Device Models that use the same register protocol, such as the Huawei SUN2000 family.
- Support exact mappings such as `145 -> Model A` without converting numeric telemetry into strings.
- Support bitmask alarm registers where multiple independent alarms can be active simultaneously.
- Show interpreted values in Device Detail and other display surfaces while retaining numeric values for calculations, charts, integrations, and exports.
- Export both numeric and interpreted values.
- Record decoded inverter alarms in the existing Alarm Log and optionally email a Plant-scoped Role.
- Preserve existing Device Model metadata, telemetry consumers, Alarm Rules, and permissions during migration.

## Non-goals

- Firmware-version-specific Register Profiles. A new Profile can be introduced later if a firmware family is found to differ.
- Replacing threshold-based Alarm Rules.
- Storing a second interpreted copy of telemetry.
- Delivering email through a persistent retry/outbox subsystem. This design reuses the existing asynchronous alarm mail path.
- Sending recovery/cleared emails. Email is sent only when a new alarm opens.
- Moving operational notes, maintenance, or curtailment records into Alarm Log. Event Logbook remains a separate feature.

## Existing System Constraints

- Telemetry is stored in `telemetry.raw_register_reading` and currently resolved through `telemetry.mapped_data_items()` for scale, offset, enablement, latest values, history, and Alarm Rule evaluation.
- `dataItemMap` is numeric and is consumed by charts, reports, SCADA, energy analysis, and Alarm Rules.
- Device-level Register Metadata currently overrides Device Model Register Metadata.
- Alarm events currently originate from threshold rules and are stored in `alarm.alarm_event`.
- Alarm email recipients are active users holding a selected Role in the relevant Plant scope.
- Registration creates a disabled, unassigned account with `email_verified_at = NULL`. Login currently maps all disabled, locked, and invalid-credential cases to the same `401` response, and the current `ACTIVE`/`DISABLED` status cannot distinguish pending approval from an Admin-disabled account.

## Chosen Architecture

Raw telemetry remains the source of truth. A Register Profile resolver becomes the single interpretation path:

```text
raw register value
    -> Modbus decoding
    -> scaled numeric value
    -> exact/bitmask display mappings
    -> decoded alarm signals
```

The resolver produces three logically separate outputs:

- numeric values for existing calculations and threshold rules;
- optional display strings for user-facing surfaces and exports;
- zero or more decoded alarm signals for ingestion-time alarm lifecycle handling.

This is preferred over frontend-only conversion because backend alarms and email need the same interpretation. It is preferred over storing interpreted telemetry because Profile changes would otherwise require historical rewrites and duplicate telemetry storage.

## Domain Model

### Register Profile

A Register Profile belongs to one Organization and has a unique name within that Organization. It represents a reusable register protocol/family, for example `Huawei SUN2000 Series`.

Each Device Model has an optional `register_profile_id`. Multiple Device Models in the same Organization may reference one Profile. Cross-Organization references are forbidden.

A Profile cannot be deleted while a Device Model references it.

### Register Profile Address

Each Profile contains Address rows. An Address owns the fields currently associated with Device Model Register Metadata:

- address key and display name;
- unit and data type;
- scale, offset, decimals, and enablement;
- notes;
- Modbus function code, register, word order, and Modbus data type;
- mapping mode: `NONE`, `EXACT`, or `BITMASK`.

Address keys are unique within a Profile.

Device-level Register Metadata remains an optional override and wins over the Profile for the existing common metadata fields. Value/alarm mappings are Profile-owned; a device requiring different mappings should use a different Profile rather than silently override the shared protocol.

### Value Mapping

A Profile Address may own multiple mapping rows:

- match value/mask;
- display value;
- `is_alarm`;
- severity when `is_alarm` is true;
- enabled state.

For `EXACT`, match values are unique for the Address. Non-alarm entries support ordinary enumerations such as `145 -> Model A`. An exact value can represent normal state by having `is_alarm = false`.

For `BITMASK`, each mapping mask must be a positive single-bit integer. Multiple mappings may match one incoming value. A missing bit means that mapping's alarm is inactive. Bitmask Addresses require scale `1`, offset `0`, and a non-negative integral decoded value.

Unknown values remain numeric, have no display mapping, and never create an alarm.

### Plant Alarm Notification Settings

Each Plant has settings for metadata-driven alarm email:

- `email_enabled`, default `false`;
- optional `notify_role_id`.

The Role must be global or belong to the Plant's Organization. Register Profiles never store Role IDs because a Profile may be reused by Plants with different operators.

Turning email off does not disable Alarm Log events or realtime delivery.

## Metadata Migration

The migration preserves all current behavior:

1. Create one Register Profile for each existing Device Model that has Register Metadata.
2. Copy that model's metadata rows into the new Profile Address table.
3. Link the Device Model to the generated Profile.
4. Leave value mappings empty, so existing numeric behavior is unchanged.
5. Keep device-level overrides and their precedence.

Generated Profile names must be deterministic and unique within the Organization. Admins may later link multiple Device Models to one shared Profile and remove the redundant unreferenced Profiles.

During rollout, API handlers and middleware configuration generation must read through one compatibility path so old model metadata and new Profiles cannot diverge. The implementation plan will sequence the schema expansion, data migration, reader switch, and cleanup as an expand/contract change.

## Telemetry Resolution

Mapping comparison uses the decoded raw register value before scale/offset. This keeps enum codes and bit masks stable. Numeric consumers continue to receive the scaled value.

Latest and history responses add a display map without changing the existing numeric map:

```json
{
  "dataItemMap": {
    "30070": 145
  },
  "displayItemMap": {
    "30070": "Model A"
  }
}
```

`displayItemMap` contains only keys with a successful mapping. Consumers fall back to formatting `dataItemMap` when a display value is absent. A bitmask with multiple matches is rendered as a stable, ordered comma-separated list.

Profile edits apply immediately to latest and historical telemetry reads because raw telemetry is unchanged and interpretation occurs at read time. This matches the system's existing current-metadata scale/offset behavior.

Alarm events are different: they snapshot the interpreted label, Profile, Device Model, Address, raw value, and severity when opened. Historical Alarm Log records therefore remain immutable in meaning after Profile edits.

## UI Display and Export

Device Detail, SCADA tables, Alarm Log, and other status-oriented surfaces use `displayItemMap` as the primary value when available. The numeric value remains available as secondary text or a tooltip. Numeric charts and calculations continue to use `dataItemMap`.

CSV and report exports include both values. For mapped points, the stable shape is two columns per point:

```text
30070_value,30070_display
145,Model A
```

Unmapped points have an empty display column. Existing columns and numeric semantics are not silently replaced.

## Metadata-Driven Alarm Lifecycle

Register alarm evaluation runs in the telemetry ingestion transaction after the reading has been stored and resolved. Threshold Alarm Rules continue to run unchanged.

Alarm events gain a source discriminator:

- `RULE` for existing threshold rules;
- `REGISTER` for Register Profile mappings.

Register-origin events identify their source by Device plus mapping identity and carry an immutable snapshot containing Profile name, Device Model, Address, raw value, decoded message, and severity. They appear in the existing Alarm Log, use existing acknowledgement behavior, and are delivered through the existing realtime alarm channel.

`alarm.alarm_event` is expanded so `alarm_rule_id` is nullable and a Register mapping source UUID can identify Register-origin events. The source UUID is an immutable copied identifier rather than a foreign key to mutable configuration, allowing mappings to be deleted while historical events retain identity through the UUID and snapshot. A database check requires a Rule reference only for `RULE` events and a Register source UUID only for `REGISTER` events. Separate partial unique indexes enforce one open event per Rule and one open event per Device/Register source UUID.

### Exact mappings

Only one exact mapping for an Address can be active at a time:

```text
0 (normal) -> no open event
145 (Over Voltage) -> open mapping 145
146 (Over Temperature) -> clear mapping 145, open mapping 146
0 (normal) -> clear mapping 146
```

Changing from one alarm code to another always creates distinct historical events.

### Bitmask mappings

Each bit has an independent lifecycle:

```text
5 (bits 0 and 2) -> open bit 0 and bit 2
4 (bit 2) -> clear bit 0; keep bit 2 open
0 -> clear bit 2
```

A partial unique constraint prevents more than one open event for the same Device and Register mapping. Repeated telemetry with the same active code/bit creates no duplicate event or email.

Disabling or deleting an alarm mapping must clear any open events sourced by that mapping in the same transaction. Historical events retain their snapshots. Deleting an assigned Profile is rejected.

## Alarm Email

When a Register-origin event opens, the service checks the Plant's notification settings after the ingestion transaction commits. If email is enabled and a Notify Role is configured, the existing recipient resolver selects active users holding that Role globally or for the Plant.

The email includes:

- Plant code/name;
- Device and Device Model;
- Register Profile and Address;
- raw value and decoded message;
- severity and observed time.

Email is sent only for newly opened events. Exact code transitions and newly set bitmask bits each count as newly opened events. Clearing an alarm does not send email.

SMTP failures are logged but do not roll back telemetry or Alarm events. A durable retry queue is outside this scope.

## Login Status Responses

Account status gains `PENDING_ACCESS` in addition to `ACTIVE` and `DISABLED`:

- self-registration creates `PENDING_ACCESS`;
- email verification updates only `email_verified_at` and does not grant access;
- Admin approval assigns Organization/Role and transitions the account to `ACTIVE`;
- disabling a previously approved account transitions it to `DISABLED`.

Existing unassigned disabled accounts are migrated to `PENDING_ACCESS`. Existing disabled accounts that already have an Organization and baseline Role remain `DISABLED`. This makes pending approval and administrative suspension deterministic instead of inferring intent during every login.

Login evaluates requests in this order:

1. normalize the identifier and enforce the existing attempt rate limit;
2. load the account or run the dummy password comparison when absent;
3. verify the password;
4. only after a valid password, classify account state;
5. create a session for an active, authorized account.

The HTTP responses are machine-readable JSON errors:

- `401 INVALID_CREDENTIALS`: unknown account or wrong password;
- `403 EMAIL_VERIFICATION_REQUIRED`: password is correct and `email_verified_at` is null;
- `403 ACCESS_PENDING`: password is correct, email is verified, and status is `PENDING_ACCESS`;
- `403 ACCOUNT_DISABLED`: password is correct and status is `DISABLED`;
- `200`: login succeeds.

Locked-account and rate-limit protections remain in force. Failed password attempts continue to be audited without exposing whether an identifier exists.

The Login UI maps those codes to Thai messages. `EMAIL_VERIFICATION_REQUIRED` shows a resend button. The other pending/disabled states provide guidance but no session.

## Resend Verification

The resend endpoint accepts an email or username identifier so it works regardless of how the user attempted login. It always returns `202 Accepted`, including for unknown, already verified, disabled, or otherwise ineligible accounts.

The endpoint is rate-limited/cooldown-protected by identifier and source IP. It only creates a replacement token and sends mail for an existing, unverified registration. The UI shows the same generic success notice for every `202` response.

## Admin Experience

The Register Metadata area becomes a three-level editor:

```text
Register Profiles
    -> Addresses
        -> Value/Alarm mappings
```

Admins can:

- create, edit, and delete unassigned Profiles;
- assign multiple Device Models to one Profile;
- edit Address and Modbus decoding fields;
- select `NONE`, `EXACT`, or `BITMASK` mapping mode;
- configure display text, alarm state, and severity;
- preview an input value to see its numeric/display/alarm result;
- import/export Profile mappings as CSV.

CSV uses one mapping per row and repeats the Profile/Address fields. An Address without mappings has one row with empty mapping columns. Import validates an entire Profile before committing changes.

The Alarm page adds Plant Notification Settings for email enablement and Notify Role. Alarm Log adds source, Address, raw value, and decoded message while preserving existing Rule event columns and actions.

## Permissions

No speculative RBAC resource is added:

- Register Profile and mapping reads/writes reuse the corresponding `device_model` permissions.
- Plant notification settings reuse `alarm:read` and `alarm:update`.
- Decoded telemetry and alarms use the same read permissions as their existing numeric/event equivalents.

All Profile, Device Model, Role, and Plant references are checked for Organization consistency at the service and database boundaries.

## Validation and Error Handling

- Profile names are unique per Organization.
- Address keys are unique per Profile.
- Exact values are unique per Address.
- Bit masks are unique, positive, and single-bit.
- Alarm mappings require a supported severity.
- Bitmask inputs must be non-negative integers and use scale `1`/offset `0`.
- Unknown/unmapped values remain numeric and do not alarm.
- A Profile referenced by any Device Model cannot be deleted.
- Disabling/removing alarm mappings clears their open events transactionally.
- Notify Roles must be visible to the Plant's Organization.
- Profile import is atomic: invalid rows reject the complete import.
- Login state is returned only after password verification.
- Resend responses do not reveal account existence.

## API Compatibility

- Existing numeric `dataItemMap` fields and endpoints remain valid.
- `displayItemMap` and Register Profile endpoints are additive.
- Existing Alarm Rules remain readable and writable without Register Profile fields.
- Alarm Event responses add source/snapshot fields while retaining current identifiers, severity, timestamps, and acknowledgement fields.
- The OpenAPI contract is updated before frontend consumers switch to the new fields.

## Testing Strategy

### Database and migration

- Existing Device Model metadata is migrated without changing resolved numeric telemetry.
- Generated Profiles are Organization-scoped and correctly linked.
- Cross-Organization Profile/Model and Plant/Role references fail.
- Uniqueness and bitmask constraints are enforced.

### Resolver

- Scale/offset behavior remains unchanged.
- Exact mappings return the expected display value.
- Unknown exact values fall back to numeric with no alarm.
- Multiple bitmask bits resolve in stable order.
- Invalid fractional/negative bitmask inputs produce no alarm and an observable evaluation error/log.
- Device-level common metadata continues to override the Profile.

### Alarm ingestion

- Exact normal -> alarm -> different alarm -> normal produces two distinct cleared events.
- Bitmask bits open and clear independently.
- Repeated active values create no duplicate events or email.
- Register events appear in Alarm Log, realtime messages, and acknowledgement flows.
- Editing/deleting an alarm mapping preserves historical snapshots and clears open events safely.

### Email

- Disabled Plant email sends nothing while events still open.
- Enabled email resolves only active recipients in the configured Role/Plant scope.
- One email is sent per newly opened exact code/bit.
- SMTP failure does not fail ingestion.

### Login and resend

- Unknown account and wrong password have indistinguishable responses.
- Unverified/access-pending/disabled codes are returned only with a correct password.
- Active accounts still create sessions normally.
- Resend accepts email and username, always returns `202`, rotates tokens only for eligible accounts, and enforces cooldown/rate limits.

### Frontend and export

- Display surfaces prefer `displayItemMap` and fall back to numeric values.
- Numeric charts and calculations continue using `dataItemMap`.
- CSV/report output includes both numeric and display columns.
- Profile mapping preview matches backend resolver behavior.
- Alarm mail controls require the existing alarm update permission.

## Delivery Sequence

The implementation should be split into independently verifiable phases:

1. Login status responses and resend-verification UX/rate limiting.
2. Register Profile schema, migration, CRUD, Device Model assignment, and compatibility reads.
3. Central resolver display mappings and additive telemetry/OpenAPI responses.
4. UI display adoption and dual-value export.
5. Register-origin Alarm Log lifecycle and event snapshots.
6. Plant-scoped email settings and Register alarm email delivery.
7. Admin Profile/mapping editor, CSV transfer, preview, and final compatibility cleanup.

Each phase must retain passing existing tests and add focused regression/integration tests before changing production behavior.

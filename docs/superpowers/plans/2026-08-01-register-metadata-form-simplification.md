# Register Metadata Form Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce the perceived duplication in the Register Metadata "Address Metadata" form (`Address/Key` vs `Modbus Register`, `Data type` vs `Modbus type`) by grouping the form into Address/Modbus/Display sections, auto-filling `Address/Key` from the Modbus register on create, and hiding `Data type` whenever a Modbus type is set — with zero backend/schema changes.

**Architecture:** Pure frontend change inside `apps/web/app/features/register-metadata/register-metadata-page.tsx`'s `AddressMetadataDialog` component (state + JSX only) and one small change to the parent list's `Type` column rendering. `addressKey` and `dataType` remain real, independently-submitted fields — nothing is removed from the API payload or the backend.

**Tech Stack:** Next.js/React (existing), the `Dialog`/`Select` primitives already in use on this page (`apps/web/app/components/ui/dialog.tsx`, `apps/web/app/components/ui/select.tsx`).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-01-register-metadata-form-simplification-design.md` — every task here traces to a section of that spec.
- No backend, database, or API contract changes — this is `apps/web` only, and only within `register-metadata-page.tsx`.
- `apps/web` has no test runner. Verification is `cd apps/web && npx tsc --noEmit` clean plus the manual checks listed per task — there is no TDD red/green cycle for this plan.
- Do not change `DeviceModelDialog` (Brand/Type/Model/Source Type) — unrelated to this spec.
- Do not add any new frontend validation for `Address/Key` collisions — the existing backend upsert-by-`addressKey` behavior is unchanged and not warned about.

---

### Task 1: Group the Address Metadata form, auto-fill Address/Key, hide Data type when Modbus type is set

**Files:**
- Modify: `apps/web/app/features/register-metadata/register-metadata-page.tsx`

**Interfaces:**
- Consumes: existing `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogBody` (`apps/web/app/components/ui/dialog.tsx`), `Select`/`SelectTrigger`/`SelectValue`/`SelectContent`/`SelectItem` (`apps/web/app/components/ui/select.tsx`), `labelClass`/`inputClass`/`primaryButtonClass`/`secondaryButtonClass` (`apps/web/app/components/ui.ts`), `MIDDLEWARE_DATA_TYPES` and `DeviceModelRegisterMetadata` (`apps/web/app/lib/types.ts`) — all already imported in this file, no new imports needed.
- Produces: no new exports — this is a self-contained component-internal change.

- [ ] **Step 1: Add the two new pieces of state to `AddressMetadataDialog`**

In `apps/web/app/features/register-metadata/register-metadata-page.tsx`, inside `AddressMetadataDialog` (currently starting at the `function AddressMetadataDialog({ model, item, onClose, onSaved })` line), add one new state variable right after the existing `const [addressKey, setAddressKey] = useState(item?.addressKey ?? "");` line:

```tsx
  const [addressKeyEdited, setAddressKeyEdited] = useState(Boolean(item));
```

(defaults to `true` when editing an existing row — matches the existing `readOnly={Boolean(item)}` behavior on the input, since auto-fill must never run in edit mode.)

- [ ] **Step 2: Add the auto-fill effect**

Add this `useEffect` right after the existing `submit` function's closing brace (before the `return (`):

```tsx
  useEffect(() => {
    if (item || addressKeyEdited) return;
    if (modbusFunctionCode === "" || modbusRegister === "") return;
    setAddressKey(`${modbusFunctionCode}:${modbusRegister}`);
  }, [item, addressKeyEdited, modbusFunctionCode, modbusRegister]);
```

This only runs in create mode (`!item`), only while the user hasn't typed into `Address/Key` themselves (`!addressKeyEdited`), and only once both Function code and Register have a value.

- [ ] **Step 3: Add the "force dataType to number while Modbus type is set" effect**

Add this `useEffect` right after the one from Step 2:

```tsx
  useEffect(() => {
    if (modbusDataType !== "") setDataType("number");
  }, [modbusDataType]);
```

- [ ] **Step 4: Mark `Address/Key` as manually edited on user input**

Find the `Address/Key` input (currently `<label className={labelClass}>Address / Key<input className={inputClass} autoFocus value={addressKey} onChange={(event) => setAddressKey(event.target.value)} maxLength={200} readOnly={Boolean(item)} required /></label>`) and change its `onChange` to also flip the new flag:

```tsx
            <label className={labelClass}>Address / Key<input className={inputClass} autoFocus value={addressKey} onChange={(event) => { setAddressKey(event.target.value); setAddressKeyEdited(true); }} maxLength={200} readOnly={Boolean(item)} required /></label>
```

- [ ] **Step 5: Regroup the form JSX into Address / Modbus Register / Display / Notes sections**

Replace the entire `<form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>...</form>` block inside `AddressMetadataDialog`'s `return` with:

```tsx
          <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
            <label className={`${labelClass} sm:col-span-2`}>Address / Key<input className={inputClass} autoFocus value={addressKey} onChange={(event) => { setAddressKey(event.target.value); setAddressKeyEdited(true); }} maxLength={200} readOnly={Boolean(item)} required /></label>

            <p className="col-span-2 text-xs font-extrabold uppercase text-ink-soft">Modbus Register</p>
            <div className="col-span-2 grid grid-cols-2 gap-4 sm:grid-cols-4">
              <Select value={modbusFunctionCode} onValueChange={setModbusFunctionCode}>
                <SelectTrigger><SelectValue placeholder="-" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="">-</SelectItem>
                  <SelectItem value="3">FC03</SelectItem>
                  <SelectItem value="4">FC04</SelectItem>
                </SelectContent>
              </Select>
              <input className={inputClass} type="number" min="0" max="65535" placeholder="Register" value={modbusRegister} onChange={(event) => setModbusRegister(event.target.value)} />
              <Select value={modbusWordOrder} onValueChange={setModbusWordOrder}>
                <SelectTrigger><SelectValue placeholder="Word order (default)" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="">Word order (default)</SelectItem>
                  <SelectItem value="HIGH_LOW">HIGH_LOW</SelectItem>
                  <SelectItem value="LOW_HIGH">LOW_HIGH</SelectItem>
                </SelectContent>
              </Select>
              <Select value={modbusDataType} onValueChange={setModbusDataType}>
                <SelectTrigger><SelectValue placeholder="Modbus type" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="">Modbus type</SelectItem>
                  {MIDDLEWARE_DATA_TYPES.map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <p className="col-span-2 -mb-2 text-xs text-ink-soft">เว้นว่างทั้งหมดถ้าเป็น display metadata อย่างเดียว ไม่ใช้ poll จริง</p>

            <p className="col-span-2 text-xs font-extrabold uppercase text-ink-soft">Display</p>
            <label className={labelClass}>Display name<input className={inputClass} value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} placeholder="Active power" /></label>
            <label className={labelClass}>Unit<input className={inputClass} value={unit} onChange={(event) => setUnit(event.target.value)} maxLength={40} placeholder="kW" /></label>
            {modbusDataType === "" && (
              <label className={labelClass}>Data type
                <Select value={dataType} onValueChange={(value) => setDataType(value as DeviceModelRegisterMetadata["dataType"])}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="number">number</SelectItem>
                    <SelectItem value="boolean">boolean</SelectItem>
                    <SelectItem value="text">text</SelectItem>
                    <SelectItem value="enum">enum</SelectItem>
                  </SelectContent>
                </Select>
              </label>
            )}
            <label className={labelClass}>Scale<input className={inputClass} type="number" step="any" value={scale} onChange={(event) => setScale(event.target.value)} required /></label>
            <label className={labelClass}>Offset<input className={inputClass} type="number" step="any" value={offset} onChange={(event) => setOffset(event.target.value)} required /></label>
            <label className={labelClass}>Decimals<input className={inputClass} type="number" min="0" max="9" value={decimals} onChange={(event) => setDecimals(event.target.value)} required /></label>
            <label className="flex items-center gap-2 self-end text-sm font-bold text-slate-800"><input className="h-4 w-4 accent-brand" type="checkbox" checked={isEnabled} onChange={(event) => setIsEnabled(event.target.checked)} /> เปิดใช้งาน</label>

            <label className={`${labelClass} sm:col-span-2`}>Notes<textarea className={`${inputClass} min-h-24 py-2`} value={notes} onChange={(event) => setNotes(event.target.value)} maxLength={500} /></label>
            {error && <p className="rounded-md bg-rose-50 px-3 py-2 text-sm font-bold text-danger sm:col-span-2">{error}</p>}
            <div className="flex justify-end gap-2 sm:col-span-2"><button type="button" className={secondaryButtonClass} onClick={onClose} disabled={pending}>ยกเลิก</button><button className={primaryButtonClass} disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก Address"}</button></div>
          </form>
```

Note what changed vs. the original: `Address / Key` moved to its own full-width row at the top (was previously the first of the two-column grid, unlabeled as a section); the four Modbus inputs are now under an explicit "Modbus Register" eyebrow label with the existing hint text moved below them; `Display name`/`Unit`/`Data type`/`Scale`/`Offset`/`Decimals`/the enabled checkbox are now under a "Display" eyebrow label; `Data type` is wrapped in `{modbusDataType === "" && (...)}` so it only renders when no Modbus type is set. Every field's `value`/`onChange`/`required`/`maxLength`/etc. props are unchanged from the original — only position and the one conditional wrapper changed.

- [ ] **Step 6: List view — show `modbusDataType` when present in the `Type` column**

In `RegisterMetadataPage`'s row rendering (the `filteredItems.map((item) => (...))` block), find:

```tsx
                <span className="hidden truncate text-ink lg:block">{item.dataType}</span>
```

Replace with:

```tsx
                <span className="hidden truncate text-ink lg:block">{item.modbusDataType || item.dataType}</span>
```

- [ ] **Step 7: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean, no errors.

- [ ] **Step 8: Manual verification**

Run `npm run dev` in `apps/web`, log in, go to Register Metadata, select any Device Model:
1. Click "เพิ่ม Address" (create). Select Function code `FC03` and type Register `40001` — confirm `Address / Key` auto-fills to `3:40001` and the `Data type` field disappears from the form.
2. Clear Function code back to `-` — confirm `Data type` reappears, pre-selected to `number`.
3. Start a new create dialog. Type directly into `Address / Key` first (e.g. `my-custom-key`), then select Function code + Register — confirm `Address / Key` does NOT get overwritten by the auto-fill.
4. Open an existing Modbus-backed row for edit — confirm `Address / Key` is still read-only and `Data type` is still hidden (since `modbusDataType` is already set from the stored row).
5. Create a display-only row (leave all 4 Modbus fields empty) — confirm `Address / Key` never auto-fills and `Data type` is visible and required as before.
6. In the address list (model selected, not in a dialog), confirm rows with a Modbus type show that type (e.g. `FLOAT32`) in the `Type` column instead of `number`, and display-only rows still show their `dataType`.

- [ ] **Step 9: Commit**

```bash
git add apps/web/app/features/register-metadata/register-metadata-page.tsx
git commit -m "$(cat <<'EOF'
Group Address Metadata form into Address/Modbus/Display sections

Auto-fills Address/Key from function code + register on create
(stops once the user types into it manually), hides the Data type
picker while a Modbus type is set (forced to "number" underneath),
and shows the Modbus type instead of the now-uniform "number" in the
list's Type column for Modbus-backed rows.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- Form structure (Address/Key, Modbus Register, Display, Notes sections) — Step 5. ✓
- Auto-fill Address/Key from function code + register, create-mode only, stops after manual edit — Steps 1, 2, 4. ✓
- Conditional Data type visibility + forced "number" underneath — Steps 3, 5. ✓
- List view Type column shows modbusDataType when present — Step 6. ✓
- No backend/schema changes — confirmed, this plan touches one frontend file only. ✓
- No new Address/Key collision validation — confirmed, not added anywhere in this plan. ✓
- `DeviceModelDialog` untouched — confirmed, this plan never touches that function. ✓

**Placeholder scan:** none found — every step has literal code, no "TBD"/"similar to"/vague instructions.

**Type consistency:** `addressKeyEdited` is a plain `boolean` used consistently in Steps 1, 2, 4 (state declaration, effect dependency, `onChange` setter) — no signature drift. `modbusDataType`/`dataType`/`modbusFunctionCode`/`modbusRegister` are all pre-existing state variables from the original file, referenced with their existing names and types throughout — none renamed or redeclared.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-01-register-metadata-form-simplification.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

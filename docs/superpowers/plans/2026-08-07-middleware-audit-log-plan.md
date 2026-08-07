# Middleware Audit Log Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** บันทึกผลการ pull telemetry ของ Middleware ลง Audit Log และเพิ่มตัวกรอง Middleware Log ในหน้าเว็บ

**Architecture:** ใช้ `audit_log` เดิมผ่าน method ของ ingestion service ที่ puller เรียกแบบ best-effort แล้วให้หน้า Audit กรอง action prefix ฝั่ง client โดยไม่เพิ่ม API หรือ dependency ใหม่

**Tech Stack:** Go, PostgreSQL migration, React/TypeScript, Go tests

## Global Constraints

- ห้ามทำให้ audit logging ที่ล้มเหลวหยุดการดึง telemetry
- ห้ามแก้ทับการเปลี่ยนแปลงที่มีอยู่ใน working tree
- ต้องมี regression test ก่อน production implementation

---

### Task 1: Add backend middleware pull audit writer

**Files:**
- Modify: `services/platform-api/internal/ingestion/service.go`
- Test: `services/platform-api/internal/ingestion/service_test.go`

**Interfaces:**
- Produces: `RecordMiddlewarePullEvent(ctx, client, action, details) error`

- [ ] Write a failing test for the event payload/action contract using the existing ingestion test patterns.
- [ ] Run `go test ./internal/ingestion -run MiddlewarePull` and verify the expected failure.
- [ ] Implement the smallest writer using `CreateAuditEventFull` with nil actor and target middleware client.
- [ ] Run the focused test and then the ingestion package tests.

### Task 2: Emit events from telemetry puller

**Files:**
- Modify: `services/platform-api/internal/telemetrypull/puller.go`
- Test: `services/platform-api/internal/telemetrypull/puller_test.go`

**Interfaces:**
- Consumes: `RecordMiddlewarePullEvent` on the `Ingester` interface.

- [ ] Add a failing test that observes successful and failed pull audit calls.
- [ ] Run the focused telemetrypull test and verify it fails for the missing calls.
- [ ] Emit started, empty, failed, succeeded, and ack_failed events while retaining existing data flow.
- [ ] Run all telemetrypull tests.

### Task 3: Add Middleware Log filter to Audit page

**Files:**
- Modify: `apps/web/app/features/audit/audit-page.tsx`

**Interfaces:**
- Consumes: existing `AuditEvent[]` API response.

- [ ] Add a local filter state and buttons for all vs Middleware Log.
- [ ] Render filtered rows and an accurate empty-state message.
- [ ] Run web typecheck/build.

### Task 4: Verify integration

**Files:**
- No source changes.

- [ ] Run focused Go tests.
- [ ] Run full Go tests for platform-api.
- [ ] Run web typecheck/build.
- [ ] Inspect git diff and confirm unrelated user changes remain untouched.

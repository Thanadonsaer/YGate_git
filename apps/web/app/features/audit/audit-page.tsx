"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { FormMessage } from "../../components/ui/form";
import { useCallback, useEffect, useState } from "react";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { DataTable, TableColumn } from "../../components/ui/data-table";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import type { AuditEvent } from "../../lib/types";

export function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<"all" | "middleware">("all");

  const loadEvents = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/admin/audit?limit=100");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู Audit Log");
      if (!response.ok) throw new Error("ไม่สามารถโหลด Audit Log ได้");
      setEvents((await response.json()) as AuditEvent[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadEvents(); }, [loadEvents]);

  const visibleEvents = filter === "middleware" ? events.filter((event) => event.action.startsWith("middleware.pull.")) : events;

  async function clearAudit() {
    const expected = "DELETE";
    if (window.prompt(`พิมพ์ ${expected} เพื่อ clear รายการ Audit ที่แสดง (หลักฐานต้นทางยังคงเป็น append-only)`) !== expected) return;
    const response = await api("/api/v1/admin/audit", {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) { toast.success("Clear Audit Log แล้ว"); await loadEvents(); }
    else setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ clear Audit view ได้" : "ไม่สามารถ clear Audit Log ได้");
  }

  return <div className="content audit-content">
    <div className="section-heading">
      <div><p>Append-only activity</p><h2>Audit Log</h2></div>
      <div className="heading-actions">
        <Button variant="icon" onClick={() => void loadEvents()} title="รีเฟรช" aria-label="รีเฟรช Audit Log"><RefreshCw size={18} /></Button>
        <Button variant={filter === "all" ? "primary" : "secondary"} compact onClick={() => setFilter("all")}>ทั้งหมด</Button>
        <Button variant={filter === "middleware" ? "primary" : "secondary"} compact onClick={() => setFilter("middleware")}>Middleware Log</Button>
        <Button variant="secondary" compact danger onClick={() => void clearAudit()}><Trash2 size={17} /> Clear Audit</Button>
      </div>
    </div>
    {error && <FormMessage>{error}</FormMessage>}
    {loading ? (
      <div className="table-state">กำลังโหลด Audit Log</div>
    ) : (
      <DataTable
        value={visibleEvents}
        dataKey="id"
        aria-label="Audit Log"
        emptyMessage={
          <div className="table-state">
            {error ? "" : filter === "middleware" ? "ยังไม่มี Middleware Log ในขอบเขตที่เข้าถึงได้" : "ยังไม่มี Audit Event ในขอบเขตที่เข้าถึงได้"}
          </div>
        }
      >
        <TableColumn field="occurredAt" header="เวลา" sortable body={(event: AuditEvent) => (
          <div className="grid gap-1"><strong>{formatDate(event.occurredAt)}</strong><small className="block text-[11px] text-ink-soft">{event.sourceIp || "ไม่ระบุ IP"}</small></div>
        )} />
        <TableColumn field="action" header="Action" sortable body={(event: AuditEvent) => (
          <div className="grid gap-1"><strong>{event.action}</strong><small className="block text-[11px] text-ink-soft">{event.correlationId || `#${event.id}`}</small></div>
        )} />
        <TableColumn header="Plant" body={(event: AuditEvent) => <span>{middlewarePlant(event.afterData)}</span>} />
        <TableColumn field="actorEmail" header="Actor" sortable body={(event: AuditEvent) => (
          <div className="grid gap-1"><span>{event.actorEmail || event.actorUserId || "system"}</span><small className="block text-[11px] text-ink-soft">{event.organizationId || "global"}</small></div>
        )} />
        <TableColumn field="targetType" header="Target" sortable body={(event: AuditEvent) => (
          <div className="grid gap-1"><span>{event.targetType}</span><small className="block text-[11px] text-ink-soft">{event.targetId || "-"}</small></div>
        )} />
        <TableColumn header="รายละเอียด" body={(event: AuditEvent) => <AuditDetails beforeData={event.beforeData} afterData={event.afterData} />} />
      </DataTable>
    )}
  </div>;
}

function middlewarePlant(value: unknown): string {
  if (!value || typeof value !== "object") return "-";
  const details = value as { plantCode?: unknown; plantName?: unknown; plants?: unknown };
  if (Array.isArray(details.plants)) {
    const labels = details.plants.map((item) => {
      if (!item || typeof item !== "object") return "";
      const plant = item as { code?: unknown; name?: unknown };
      const code = typeof plant.code === "string" ? plant.code : "";
      const name = typeof plant.name === "string" ? plant.name : "";
      return name && code ? `${name} (${code})` : name || code;
    }).filter(Boolean);
    if (labels.length > 0) return labels.join(", ");
  }
  const code = typeof details.plantCode === "string" ? details.plantCode : "";
  const name = typeof details.plantName === "string" ? details.plantName : "";
  return name && code ? `${name} (${code})` : name || code || "-";
}

function AuditDetails({ beforeData, afterData }: { beforeData?: unknown; afterData?: unknown }) {
  if (beforeData == null && afterData == null) return <span className="muted-text">-</span>;
  return <details className="audit-json">
    <summary>ดูข้อมูล</summary>
    {beforeData != null && <><strong>Before</strong><pre>{JSON.stringify(beforeData, null, 2)}</pre></>}
    {afterData != null && <><strong>After</strong><pre>{JSON.stringify(afterData, null, 2)}</pre></>}
  </details>;
}

"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { api, csrfToken, formatDate } from "../../lib/api";
import type { AuditEvent } from "../../lib/types";

export function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadEvents = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/admin/audit?limit=100");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู Audit Log");
      if (!response.ok) throw new Error("ไม่สามารถโหลด Audit Log ได้");
      setEvents((await response.json()) as AuditEvent[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadEvents(); }, [loadEvents]);

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
        <Button variant="secondary" compact danger onClick={() => void clearAudit()}><Trash2 size={17} /> Clear Audit</Button>
      </div>
    </div>
    {error && <p className="form-message error">{error}</p>}
    <div className="audit-table" role="table" aria-label="Audit Log">
      <div className="audit-row audit-head" role="row"><span>เวลา</span><span>Action</span><span>Actor</span><span>Target</span><span>รายละเอียด</span></div>
      {!loading && events.map((event) => (
        <div className="audit-row" role="row" key={event.id}>
          <div><strong>{formatDate(event.occurredAt)}</strong><small>{event.sourceIp || "ไม่ระบุ IP"}</small></div>
          <div><strong>{event.action}</strong><small>{event.correlationId || `#${event.id}`}</small></div>
          <div><span>{event.actorEmail || event.actorUserId || "system"}</span><small>{event.organizationId || "global"}</small></div>
          <div><span>{event.targetType}</span><small>{event.targetId || "-"}</small></div>
          <AuditDetails beforeData={event.beforeData} afterData={event.afterData} />
        </div>
      ))}
      {loading && <div className="table-state">กำลังโหลด Audit Log</div>}
      {!loading && !error && events.length === 0 && <div className="table-state">ยังไม่มี Audit Event ในขอบเขตที่เข้าถึงได้</div>}
    </div>
  </div>;
}

function AuditDetails({ beforeData, afterData }: { beforeData?: unknown; afterData?: unknown }) {
  if (beforeData == null && afterData == null) return <span className="muted-text">-</span>;
  return <details className="audit-json">
    <summary>ดูข้อมูล</summary>
    {beforeData != null && <><strong>Before</strong><pre>{JSON.stringify(beforeData, null, 2)}</pre></>}
    {afterData != null && <><strong>After</strong><pre>{JSON.stringify(afterData, null, 2)}</pre></>}
  </details>;
}

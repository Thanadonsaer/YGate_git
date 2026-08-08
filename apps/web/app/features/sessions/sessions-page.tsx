"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { FormMessage, StatusTag } from "../../components/ui/form";
import { useCallback, useEffect, useState } from "react";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { DataTable, TableColumn } from "../../components/ui/data-table";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import type { Session } from "../../lib/types";

export function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadSessions = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/auth/sessions");
      if (!response.ok) throw new Error("ไม่สามารถโหลดเซสชันได้");
      setSessions((await response.json()) as Session[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadSessions(); }, [loadSessions]);

  async function revokeSession(session: Session) {
    const response = await api(`/api/v1/auth/sessions/${encodeURIComponent(session.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    if (response.ok) { toast.success("ยกเลิกเซสชันแล้ว"); await loadSessions(); }
    else setError("ไม่สามารถยกเลิกเซสชันได้");
  }

  async function clearSessions() {
    const expected = "DELETE";
    if (window.prompt(`พิมพ์ ${expected} เพื่อลบประวัติเซสชันทั้งหมด รวมถึงเซสชันนี้`) !== expected) return;
    const response = await api("/api/v1/auth/sessions", {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) window.location.assign("/");
    else setError("ไม่สามารถ clear เซสชันได้");
  }

  return (
    <div className="content sessions-content">
      <div className="section-heading">
        <div><p>ความปลอดภัย</p><h2>เซสชันที่เข้าสู่ระบบ</h2></div>
        <div className="heading-actions">
          <Button variant="icon" onClick={() => void loadSessions()} title="รีเฟรช" aria-label="รีเฟรชรายการเซสชัน"><RefreshCw size={18} /></Button>
          <Button variant="secondary" compact danger onClick={() => void clearSessions()}><Trash2 size={17} /> Clear ทั้งหมด</Button>
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      <DataTable value={sessions} dataKey="id" aria-label="เซสชัน" emptyMessage={<div className="table-state">{!loading && !error ? "ไม่พบเซสชัน" : ""}</div>}>
        <TableColumn field="userAgent" header="อุปกรณ์" sortable body={(session: Session) => (
          <div className="grid gap-1"><strong>{session.userAgent || "Unknown client"}</strong><small className="block text-[11px] text-ink-soft">{session.clientIp || "ไม่ระบุ IP"}</small></div>
        )} />
        <TableColumn field="lastSeenAt" header="ใช้งานล่าสุด" sortable body={(session: Session) => (
          <div className="grid gap-1"><span>{formatDate(session.lastSeenAt)}</span><small className="block text-[11px] text-ink-soft">สร้างเมื่อ {formatDate(session.createdAt)}</small></div>
        )} />
        <TableColumn header="สถานะ" body={(session: Session) => (
          session.revokedAt ? <StatusTag tone="revoked">ยกเลิกแล้ว</StatusTag> : session.current ? <StatusTag tone="current">เซสชันนี้</StatusTag> : <StatusTag tone="active">ใช้งานอยู่</StatusTag>
        )} />
        <TableColumn header="" body={(session: Session) => (
          <Button variant="icon" danger disabled={Boolean(session.revokedAt)} onClick={() => void revokeSession(session)} title="ยกเลิกเซสชัน" aria-label="ยกเลิกเซสชัน"><Trash2 size={17} /></Button>
        )} />
      </DataTable>
      {loading && <div className="table-state">กำลังโหลดเซสชัน</div>}
    </div>
  );
}

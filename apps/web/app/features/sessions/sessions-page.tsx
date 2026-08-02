"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { api, csrfToken, formatDate } from "../../lib/api";
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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
      {error && <p className="form-message error">{error}</p>}
      <div className="session-table" role="table" aria-label="เซสชัน">
        <div className="session-row session-head" role="row"><span>อุปกรณ์</span><span>ใช้งานล่าสุด</span><span>สถานะ</span><span aria-label="คำสั่ง" /></div>
        {sessions.map((session) => (
          <div className="session-row" role="row" key={session.id}>
            <div><strong>{session.userAgent || "Unknown client"}</strong><small>{session.clientIp || "ไม่ระบุ IP"}</small></div>
            <div><span>{formatDate(session.lastSeenAt)}</span><small>สร้างเมื่อ {formatDate(session.createdAt)}</small></div>
            <div>{session.revokedAt ? <span className="status revoked">ยกเลิกแล้ว</span> : session.current ? <span className="status current">เซสชันนี้</span> : <span className="status active">ใช้งานอยู่</span>}</div>
            <Button variant="icon" danger disabled={Boolean(session.revokedAt)} onClick={() => void revokeSession(session)} title="ยกเลิกเซสชัน" aria-label="ยกเลิกเซสชัน"><Trash2 size={17} /></Button>
          </div>
        ))}
        {loading && <div className="table-state">กำลังโหลดเซสชัน</div>}
        {!loading && !error && sessions.length === 0 && <div className="table-state">ไม่พบเซสชัน</div>}
      </div>
    </div>
  );
}

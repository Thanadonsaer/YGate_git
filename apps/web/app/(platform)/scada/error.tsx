"use client";

import { RefreshCw } from "lucide-react";
import { Button } from "../../components/ui/button";

// Covers /scada and /scada/live. Without it a render throw falls through to the
// framework's blank "This page couldn't load", which names neither the failure
// nor the screen that caused it -- useless for a control room and for triage.
export default function ScadaError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <div className="content scada-content">
      <div className="section-heading"><div><p>SCADA</p><h2>หน้านี้โหลดไม่สำเร็จ</h2></div></div>
      <div className="table-state" style={{ display: "grid", gap: "0.75rem", justifyItems: "start" }}>
        <p>เกิดข้อผิดพลาดขณะแสดงผล SCADA — screen ที่เลือกอาจมี design ที่บันทึกไว้ไม่สมบูรณ์</p>
        <code style={{ whiteSpace: "pre-wrap", wordBreak: "break-word" }}>{error.message || "Unknown error"}</code>
        {error.digest && <small className="muted-text">digest {error.digest}</small>}
        <Button compact onClick={reset}><RefreshCw size={16} /> ลองใหม่</Button>
      </div>
    </div>
  );
}

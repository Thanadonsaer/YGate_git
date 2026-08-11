"use client";

import { Download, FileBarChart, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { api, csrfToken, downloadBlob, errorMessage } from "../../lib/api";
import type { Plant } from "../../lib/types";
import { Button } from "../../components/ui/button";
import { StatusTag } from "../../components/ui/form";
import { toast } from "../../components/ui/sonner";

type ReportType = "EXECUTIVE_PORTFOLIO" | "OPERATIONAL_STATUS" | "FAULTS_MAINTENANCE";

function localDateTime(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function ReportsPage() {
  const now = new Date();
  const [plants, setPlants] = useState<Plant[]>([]);
  const [reportType, setReportType] = useState<ReportType>("EXECUTIVE_PORTFOLIO");
  const [plantId, setPlantId] = useState("");
  const [from, setFrom] = useState(localDateTime(new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)));
  const [to, setTo] = useState(localDateTime(now));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void api("/api/v1/plants").then(async (response) => {
      if (!response.ok) { setError("ไม่สามารถโหลดรายชื่อโรงไฟฟ้าได้"); return; }
      setPlants((await response.json()) as Plant[]);
    });
  }, []);

  async function exportReport() {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/reports/export", {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ reportType, plantIds: plantId ? [plantId] : [], from: new Date(from).toISOString(), to: new Date(to).toISOString() }),
      });
      if (!response.ok) throw new Error(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์สร้างรายงาน" : "ไม่สามารถสร้างรายงานได้");
      downloadBlob(await response.blob(), `ygate-${reportType.toLowerCase()}-${new Date().toISOString().slice(0, 10)}.xlsx`);
      toast.success("ดาวน์โหลดรายงานแล้ว");
    } catch (cause) { setError(errorMessage(cause)); }
    finally { setLoading(false); }
  }

  return <section className="feature-page reports-page">
    <div className="page-heading"><div><p className="eyebrow">TOR Reporting</p><h1>Reports</h1><p className="muted">สร้างรายงาน Portfolio, Operational และ Faults & Maintenance จากข้อมูลจริงในระบบ</p></div><FileBarChart size={28} /></div>
    <div className="report-layout">
      <div className="panel report-form">
        <div className="panel-heading"><div><h2>สร้างรายงาน</h2><p className="muted">เลือกขอบเขตและช่วงเวลาที่ต้องการ</p></div></div>
        <label>ประเภทรายงาน<select className="ea-input" value={reportType} onChange={(event) => setReportType(event.target.value as ReportType)}><option value="EXECUTIVE_PORTFOLIO">Executive Portfolio</option><option value="OPERATIONAL_STATUS">Operational Status</option><option value="FAULTS_MAINTENANCE">Faults & Maintenance</option></select></label>
        <label>โรงไฟฟ้า<select className="ea-input" value={plantId} onChange={(event) => setPlantId(event.target.value)}><option value="">ทุกโรงไฟฟ้าที่มีสิทธิ์</option>{plants.map((plant) => <option key={plant.id} value={plant.id}>{plant.code} — {plant.name}</option>)}</select></label>
        <div className="report-date-grid"><label>เริ่มต้น<input className="ea-input" type="datetime-local" value={from} onChange={(event) => setFrom(event.target.value)} /></label><label>สิ้นสุด<input className="ea-input" type="datetime-local" value={to} onChange={(event) => setTo(event.target.value)} /></label></div>
        {error && <p className="form-error">{error}</p>}
        <Button onClick={() => void exportReport()} disabled={loading}>{loading ? <RefreshCw className="spin" size={16} /> : <Download size={16} />} {loading ? "กำลังสร้าง..." : "ดาวน์โหลด XLSX"}</Button>
      </div>
      <div className="panel report-scope"><div className="panel-heading"><div><h2>ขอบเขตข้อมูล</h2><p className="muted">ระบบจะใช้ข้อมูล Plant และ Event Logbook ตามสิทธิ์ของผู้ใช้งาน</p></div></div><div className="report-stat"><strong>{plants.length}</strong><span>Plants ที่เข้าถึงได้</span></div><div className="report-types"><StatusTag tone="info">Executive</StatusTag><StatusTag tone="success">Operational</StatusTag><StatusTag tone="warning">Faults & Maintenance</StatusTag></div></div>
    </div>
  </section>;
}

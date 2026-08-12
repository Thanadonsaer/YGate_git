"use client";

import { ChartLine, Download, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { api, csrfToken, downloadBlob, errorMessage, toDatetimeLocal } from "../../lib/api";
import type { Plant } from "../../lib/types";
import { Button } from "../../components/ui/button";
import { FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { toast } from "../../components/ui/sonner";

type ReportType = "EXECUTIVE_PORTFOLIO" | "OPERATIONAL_STATUS" | "FAULTS_MAINTENANCE";

/** Sentinel for "no plant filter": Dropdown treats "" as nothing selected. */
const ALL_PLANTS = "__all__";

export function ReportsPage() {
  const now = new Date();
  const [plants, setPlants] = useState<Plant[]>([]);
  const [reportType, setReportType] = useState<ReportType>("EXECUTIVE_PORTFOLIO");
  const [plantId, setPlantId] = useState(ALL_PLANTS);
  const [from, setFrom] = useState(toDatetimeLocal(new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)));
  const [to, setTo] = useState(toDatetimeLocal(now));
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
        body: JSON.stringify({
          reportType,
          plantIds: plantId === ALL_PLANTS ? [] : [plantId],
          from: new Date(from).toISOString(),
          to: new Date(to).toISOString(),
        }),
      });
      if (!response.ok) throw new Error(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์สร้างรายงาน" : "ไม่สามารถสร้างรายงานได้");
      downloadBlob(await response.blob(), `ygate-${reportType.toLowerCase()}-${new Date().toISOString().slice(0, 10)}.xlsx`);
      toast.success("ดาวน์โหลดรายงานแล้ว");
    } catch (cause) { setError(errorMessage(cause)); }
    finally { setLoading(false); }
  }

  return (
    <div className="content reports-content">
      <div className="section-heading">
        <div><p>TOR Reporting</p><h2>Reports</h2></div>
        <div className="heading-actions">
          <Button onClick={() => void exportReport()} disabled={loading}>
            {loading ? <RefreshCw className="spin" size={16} /> : <Download size={16} />}
            {loading ? "กำลังสร้าง..." : "ดาวน์โหลด XLSX"}
          </Button>
        </div>
      </div>

      {error && <FormMessage>{error}</FormMessage>}

      <div className="report-layout">
        <section className="ea-panel report-form">
          <div className="ea-panel-head">
            <h3>สร้างรายงาน</h3>
            <span className="ts-unit">เลือกขอบเขตและช่วงเวลาที่ต้องการ</span>
          </div>

          <label className="ea-field">ประเภทรายงาน
            <Select value={reportType} onValueChange={(value) => setReportType(value as ReportType)}>
              <SelectTrigger aria-label="ประเภทรายงาน"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="EXECUTIVE_PORTFOLIO">Executive Portfolio</SelectItem>
                <SelectItem value="OPERATIONAL_STATUS">Operational Status</SelectItem>
                <SelectItem value="FAULTS_MAINTENANCE">Faults &amp; Maintenance</SelectItem>
              </SelectContent>
            </Select>
          </label>

          <label className="ea-field">โรงไฟฟ้า
            <Select value={plantId} onValueChange={setPlantId}>
              <SelectTrigger aria-label="โรงไฟฟ้า"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL_PLANTS}>ทุกโรงไฟฟ้าที่มีสิทธิ์</SelectItem>
                {plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.code} — {plant.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </label>

          <div className="report-date-grid">
            <label className="ea-field">เริ่มต้น
              <TextInput className="ea-input" type="datetime-local" value={from} onChange={(event) => setFrom(event.target.value)} />
            </label>
            <label className="ea-field">สิ้นสุด
              <TextInput className="ea-input" type="datetime-local" value={to} onChange={(event) => setTo(event.target.value)} />
            </label>
          </div>
        </section>

        <section className="ea-panel report-scope">
          <div className="ea-panel-head">
            <h3>ขอบเขตข้อมูล</h3>
            <ChartLine size={18} />
          </div>
          <p className="muted-text">ระบบจะใช้ข้อมูล Plant และ Event Logbook ตามสิทธิ์ของผู้ใช้งาน</p>
          <div className="report-stat"><strong>{plants.length}</strong><span>Plants ที่เข้าถึงได้</span></div>
          {/* Tones must be names globals.css actually defines (.status.<tone>);
              info/success/warning have no rule and rendered as bare text. */}
          <div className="report-types">
            <StatusTag tone="active">Executive</StatusTag>
            <StatusTag tone="online">Operational</StatusTag>
            <StatusTag tone="degraded">Faults &amp; Maintenance</StatusTag>
          </div>
        </section>
      </div>
    </div>
  );
}

"use client";

import { Building2, ChartLine, Cpu, GripVertical, Pencil, RefreshCw, RotateCcw, Save, Settings2, Trash2, Upload, Wifi, WifiOff, X } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { Responsive, useContainerWidth, type Layout, type ResponsiveLayouts } from "react-grid-layout";
import { api, csrfToken, formatDate } from "../../lib/api";
import type { DashboardLayout, DashboardLayoutItem, DashboardLayouts, DashboardOverview, DashboardPlantStatus, DashboardWidget, DashboardWidgetConfigs, Device, LatestTelemetry, Plant, PublishedDashboardLayout, TelemetryHistoryPage, TimeseriesWidgetConfig } from "../../lib/types";
import { usePlatformSession } from "../../components/platform-shell";

function dashboardStatusLabel(status: DashboardPlantStatus["communicationStatus"]) {
  return { ONLINE: "Online", DEGRADED: "Degraded", OFFLINE: "Offline", NO_DEVICES: "No devices", DISABLED: "Disabled" }[status];
}

export function OverviewPage() {
  const { user, liveState, lastLiveAt } = usePlatformSession();
  const [dashboard, setDashboard] = useState<DashboardOverview | null>(null);
  const [dashboardError, setDashboardError] = useState("");

  const loadDashboard = useCallback(async () => {
    try {
      const response = await api("/api/v1/dashboard/overview?staleAfterSeconds=300");
      if (!response.ok) throw new Error(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์ดูข้อมูล Device" : "ไม่สามารถโหลดภาพรวมระบบได้");
      setDashboard((await response.json()) as DashboardOverview);
      setDashboardError("");
    } catch (cause) {
      setDashboardError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    }
  }, []);

  useEffect(() => {
    void loadDashboard();
    const timer = window.setInterval(() => void loadDashboard(), 15000);
    return () => window.clearInterval(timer);
  }, [loadDashboard]);

  return (
    <div className="content overview-content">
      <DashboardCanvas dashboard={dashboard} dashboardError={dashboardError} onRefresh={loadDashboard} />
      <section className="activity-band">
        <div><span className={`signal ${liveState}`} /><div><strong>Realtime connection</strong><p>WebSocket /api/v1/realtime</p></div></div>
        <time>{lastLiveAt ? `อัปเดตล่าสุด ${formatDate(lastLiveAt)}` : "รอข้อความแรก"}</time>
      </section>
      <section className="account-section">
        <div className="section-heading"><div><p>บัญชีผู้ใช้</p><h2>{user.displayName}</h2></div></div>
        <dl className="account-grid">
          <div><dt>อีเมล</dt><dd>{user.email}</dd></div>
          <div><dt>User ID</dt><dd>{user.id}</dd></div>
          <div><dt>Organization ID</dt><dd>{user.organizationId ?? "-"}</dd></div>
        </dl>
      </section>
    </div>
  );
}

function DashboardCanvas({ dashboard, dashboardError, onRefresh }: { dashboard: DashboardOverview | null; dashboardError: string; onRefresh: () => Promise<void> }) {
  const [saved, setSaved] = useState<DashboardLayout | null>(null);
  const [published, setPublished] = useState<PublishedDashboardLayout | null>(null);
  const [layouts, setLayouts] = useState<DashboardLayouts | null>(null);
  const [widgetConfigs, setWidgetConfigs] = useState<DashboardWidgetConfigs>({});
  const [configEditorOpen, setConfigEditorOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const { width, containerRef } = useContainerWidth({ initialWidth: 1100 });

  const loadLayouts = useCallback(async () => {
    const [draftResponse, publishedResponse] = await Promise.all([
      api("/api/v1/dashboard/layout"),
      api("/api/v1/dashboard/layout/published"),
    ]);
    if (!draftResponse.ok || !publishedResponse.ok) throw new Error(draftResponse.status === 403 || publishedResponse.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์ดู Dashboard" : "ไม่สามารถโหลด Dashboard layout ได้");
    const draft = (await draftResponse.json()) as DashboardLayout;
    const live = (await publishedResponse.json()) as PublishedDashboardLayout;
    setSaved(draft);
    setPublished(live);
    setLayouts(normalizeDashboardLayouts(live.layouts));
    setWidgetConfigs(live.widgetConfigs ?? {});
    return { draft, live };
  }, []);

  useEffect(() => {
    void loadLayouts().catch((cause: unknown) => setMessage(cause instanceof Error ? cause.message : "ไม่สามารถโหลด Dashboard layout ได้"));
  }, [loadLayouts]);

  async function saveLayout() {
    if (!saved || !layouts) return;
    setPending(true);
    setMessage("");
    try {
      const response = await api("/api/v1/dashboard/layout", {
        method: "PUT",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ version: saved.version, layouts: normalizeDashboardLayouts(layouts), widgetConfigs }),
      });
      if (response.status === 409) {
        await loadLayouts();
        throw new Error("Layout ถูกแก้ไขจาก session อื่น ระบบโหลดข้อมูลล่าสุดให้แล้ว");
      }
      if (!response.ok) throw new Error(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์แก้ไข Dashboard" : "ไม่สามารถบันทึก Dashboard layout ได้");
      const next = (await response.json()) as DashboardLayout;
      setSaved(next);
      if (published) {
        setLayouts(normalizeDashboardLayouts(published.layouts));
        setWidgetConfigs(published.widgetConfigs ?? {});
      }
      setEditing(false);
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "ไม่สามารถบันทึก Dashboard layout ได้");
    } finally {
      setPending(false);
    }
  }

  async function publishLayout() {
    if (!saved || !published) return;
    setPending(true);
    setMessage("");
    try {
      const response = await api("/api/v1/dashboard/layout/publish", {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ version: saved.version, publishedVersion: published.version }),
      });
      if (response.status === 409) {
        await loadLayouts();
        throw new Error("Dashboard ถูกแก้ไขจาก session อื่น ระบบโหลดข้อมูลล่าสุดให้แล้ว");
      }
      if (!response.ok) throw new Error(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เผยแพร่ Dashboard" : "ไม่สามารถเผยแพร่ Dashboard ได้");
      const next = (await response.json()) as PublishedDashboardLayout;
      setPublished(next);
      setLayouts(normalizeDashboardLayouts(next.layouts));
      setWidgetConfigs(next.widgetConfigs ?? {});
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "ไม่สามารถเผยแพร่ Dashboard ได้");
    } finally {
      setPending(false);
    }
  }

  function beginEditing() {
    if (!saved) return;
    setLayouts(normalizeDashboardLayouts(saved.layouts));
    setWidgetConfigs(saved.widgetConfigs ?? {});
    setEditing(true);
    setMessage("");
  }

  function cancelEditing() {
    if (published) setLayouts(normalizeDashboardLayouts(published.layouts));
    if (published) setWidgetConfigs(published.widgetConfigs ?? {});
    setEditing(false);
    setMessage("");
  }

  const attentionDevices = (dashboard?.staleDeviceCount ?? 0) + (dashboard?.offlineDeviceCount ?? 0);
  const hasUnpublishedChanges = Boolean(saved && published && saved.version > published.sourceVersion);
  const chartEnabled = Boolean(layouts?.lg.some((item) => item.i === "timeseries-line"));
  const kpis: Record<string, { label: string; value: string; tone: "good" | "bad" | "warn" | "neutral"; icon: React.ReactNode }> = {
    "plant-count": { label: "โรงไฟฟ้า", value: dashboard ? dashboard.plantCount.toLocaleString() : "-", tone: "neutral", icon: <Building2 size={20} /> },
    "active-device-count": { label: "Active devices", value: dashboard ? dashboard.activeDeviceCount.toLocaleString() : "-", tone: "neutral", icon: <Cpu size={20} /> },
    "reporting-device-count": { label: "Reporting", value: dashboard ? dashboard.reportingDeviceCount.toLocaleString() : "-", tone: !dashboard || dashboard.activeDeviceCount === 0 ? "neutral" : dashboard.reportingDeviceCount === dashboard.activeDeviceCount ? "good" : "warn", icon: <Wifi size={20} /> },
    "attention-device-count": { label: "ต้องตรวจสอบ", value: dashboard ? attentionDevices.toLocaleString() : "-", tone: !dashboard ? "neutral" : attentionDevices > 0 ? "bad" : "good", icon: <WifiOff size={20} /> },
  };

  function renderWidget(widget: DashboardWidget) {
    if (widget.type === "timeseries.line") {
      return <TimeseriesWidget config={widgetConfigs["timeseries-line"]} refreshKey={dashboard?.generatedAt} editing={editing} onConfigure={() => setConfigEditorOpen(true)} />;
    }
    if (widget.type !== "plant-status") {
      const kpi = kpis[widget.type];
      if (!kpi) return null;
      return <div className={`dashboard-kpi ${kpi.tone}`}><span>{kpi.icon}</span><div><p>{kpi.label}</p><strong>{kpi.value}</strong></div></div>;
    }
    return (
      <div className="dashboard-widget-body">
        {dashboardError && <p className="form-message error">{dashboardError}</p>}
        <div className="dashboard-table" role="table" aria-label="สถานะการรับข้อมูลแต่ละโรงไฟฟ้า">
          <div className="dashboard-row dashboard-head" role="row"><span>โรงไฟฟ้า</span><span>Device</span><span>Reporting</span><span>ข้อมูลล่าสุด</span><span>สถานะ</span></div>
          {dashboard?.plants.map((plant) => (
            <div className="dashboard-row" role="row" key={plant.plantId}>
              <div><strong>{plant.name}</strong><small>{plant.code} · {plant.timezone}</small></div>
              <div><span>{plant.activeDeviceCount.toLocaleString()} active</span><small>{plant.deviceCount.toLocaleString()} ทั้งหมด</small></div>
              <div><span>{plant.reportingDeviceCount.toLocaleString()}</span><small>{plant.staleDeviceCount} stale · {plant.offlineDeviceCount} offline</small></div>
              <div><span>{plant.lastObservedAt ? formatDate(plant.lastObservedAt) : "-"}</span><small>เกณฑ์ stale {dashboard.staleAfterSeconds} วินาที</small></div>
              <span className={`status ${plant.communicationStatus.toLowerCase()}`}>{dashboardStatusLabel(plant.communicationStatus)}</span>
            </div>
          ))}
          {!dashboard && !dashboardError && <div className="table-state">กำลังโหลดข้อมูล</div>}
          {dashboard && dashboard.plants.length === 0 && <div className="table-state">ยังไม่มีโรงไฟฟ้าในขอบเขตที่เข้าถึงได้</div>}
        </div>
      </div>
    );
  }

  return (
    <section className={editing ? "dashboard-canvas editing" : "dashboard-canvas"}>
      <div className="section-heading dashboard-toolbar">
        <div><p>My dashboard</p><h2>ภาพรวมการดำเนินงาน</h2></div>
        <div className="heading-actions">
          {published && <span className={hasUnpublishedChanges ? "dashboard-version draft" : "dashboard-version"}>{hasUnpublishedChanges ? `Draft v${saved?.version}` : `Published v${published.version}`}</span>}
          <button className="icon-button" onClick={() => { void onRefresh(); if (!editing) void loadLayouts(); }} disabled={pending} title="รีเฟรช" aria-label="รีเฟรชภาพรวม"><RefreshCw size={18} /></button>
          {editing ? <>
            {chartEnabled ? <button className="secondary-button compact" onClick={() => {
              setLayouts((current) => current ? {
                lg: current.lg.filter((item) => item.i !== "timeseries-line"),
                md: current.md.filter((item) => item.i !== "timeseries-line"),
                sm: current.sm.filter((item) => item.i !== "timeseries-line"),
              } : current);
              setWidgetConfigs({});
            }} disabled={pending}><Trash2 size={17} /> ลบกราฟ</button> : <button className="secondary-button compact" onClick={() => setConfigEditorOpen(true)} disabled={pending}><ChartLine size={17} /> เพิ่มกราฟ</button>}
            <button className="secondary-button compact" onClick={cancelEditing} disabled={pending}><RotateCcw size={17} /> ยกเลิก</button>
            <button className="primary-button compact" onClick={() => void saveLayout()} disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึก"}</button>
          </> : <>
            {saved?.canPublish && hasUnpublishedChanges && <button className="primary-button compact" onClick={() => void publishLayout()} disabled={pending}><Upload size={17} /> {pending ? "กำลังเผยแพร่" : "เผยแพร่"}</button>}
            {saved?.canEdit && <button className="secondary-button compact" onClick={beginEditing} disabled={pending}><Pencil size={17} /> ปรับ Layout</button>}
          </>}
        </div>
      </div>
      {message && <p className="form-message error">{message}</p>}
      <div ref={containerRef} className="dashboard-grid-container">
        {saved && published && layouts ? (
          <Responsive<"lg" | "md" | "sm">
            width={width}
            layouts={layouts as ResponsiveLayouts<"lg" | "md" | "sm">}
            breakpoints={{ lg: 1100, md: 700, sm: 0 }}
            cols={{ lg: 12, md: 8, sm: 4 }}
            rowHeight={45}
            margin={[14, 14]}
            containerPadding={[0, 0]}
            dragConfig={{ enabled: editing, bounded: true, handle: ".widget-handle" }}
            resizeConfig={{ enabled: editing, handles: ["se"] }}
            onLayoutChange={(_layout: Layout, next: ResponsiveLayouts<"lg" | "md" | "sm">) => {
              if (editing) setLayouts(normalizeDashboardLayouts(next));
            }}
          >
            {(editing ? saved.widgets : published.widgets).filter((widget) => layouts.lg.some((item) => item.i === widget.id)).map((widget) => (
              <article key={widget.id} className={`dashboard-widget ${widget.type === "plant-status" ? "plant-status-widget" : widget.type === "timeseries.line" ? "timeseries-line-widget" : "kpi-widget"}`}>
                <header className="widget-handle"><GripVertical size={16} /><span>{widget.title}</span></header>
                {renderWidget(widget)}
              </article>
            ))}
          </Responsive>
        ) : !message ? <div className="table-state">กำลังโหลด Dashboard</div> : null}
      </div>
      {configEditorOpen && <TimeseriesConfigEditor
        initial={widgetConfigs["timeseries-line"]}
        onClose={() => setConfigEditorOpen(false)}
        onSave={(config) => {
          if (!chartEnabled) {
            setLayouts((current) => current ? addTimeseriesLayout(current) : current);
          }
          setWidgetConfigs({ "timeseries-line": config });
          setConfigEditorOpen(false);
        }}
      />}
    </section>
  );
}

function addTimeseriesLayout(layouts: DashboardLayouts): DashboardLayouts {
  const sizes = { lg: { w: 12, h: 5 }, md: { w: 8, h: 5 }, sm: { w: 4, h: 6 } } as const;
  const append = (breakpoint: keyof DashboardLayouts) => {
    const items = layouts[breakpoint];
    const y = items.reduce((bottom, item) => Math.max(bottom, item.y + item.h), 0);
    return [...items, { i: "timeseries-line", x: 0, y, ...sizes[breakpoint] }];
  };
  return { lg: append("lg"), md: append("md"), sm: append("sm") };
}

function TimeseriesWidget({ config, refreshKey, editing, onConfigure }: { config?: TimeseriesWidgetConfig; refreshKey?: string; editing: boolean; onConfigure: () => void }) {
  const [values, setValues] = useState<Array<{ at: string; value: number }>>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!config) {
      setValues([]);
      return;
    }
    const controller = new AbortController();
    void (async () => {
      const to = new Date();
      const from = new Date(to.getTime() - config.dataBinding.timeRangeHours * 60 * 60 * 1000);
      const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), limit: "500" });
      try {
        const response = await api(`/api/v1/plants/${encodeURIComponent(config.dataBinding.plantId)}/devices/${encodeURIComponent(config.dataBinding.deviceId)}/telemetry/history?${query}`, { signal: controller.signal });
        if (!response.ok) throw new Error(response.status === 404 ? "ไม่พบ Plant หรือ Device ที่ผูกไว้" : "ไม่สามารถโหลดข้อมูลกราฟได้");
        const page = (await response.json()) as TelemetryHistoryPage;
        setValues(page.data.flatMap((reading) => {
          const value = reading.dataItemMap[config.dataBinding.pointKey];
          return Number.isFinite(value) ? [{ at: reading.observedAt, value }] : [];
        }).reverse());
        setError("");
      } catch (cause) {
        if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "ไม่สามารถโหลดข้อมูลกราฟได้");
      }
    })();
    return () => controller.abort();
  }, [config, refreshKey]);

  if (!config) return <div className="timeseries-empty"><ChartLine size={24} /><span>ยังไม่ได้ตั้งค่ากราฟ</span>{editing && <button className="secondary-button compact" onClick={onConfigure}><Settings2 size={16} /> ตั้งค่า</button>}</div>;
  const numbers = values.map((point) => point.value);
  const minimum = numbers.length ? Math.min(...numbers) : 0;
  const maximum = numbers.length ? Math.max(...numbers) : 0;
  const range = maximum - minimum || 1;
  const polyline = values.map((point, index) => `${values.length === 1 ? 50 : index * 100 / (values.length - 1)},${36 - (point.value - minimum) * 32 / range}`).join(" ");
  const format = (value: number) => `${value.toFixed(config.display.decimals)}${config.display.unit ? ` ${config.display.unit}` : ""}`;

  return <div className="dashboard-widget-body timeseries-widget-body">
    <div className="timeseries-meta">
      <div><strong>{config.dataBinding.pointKey}</strong><small>{config.dataBinding.timeRangeHours}h · {values.length} points</small></div>
      {editing && <button className="icon-button" onClick={onConfigure} title="ตั้งค่ากราฟ" aria-label="ตั้งค่ากราฟ"><Settings2 size={17} /></button>}
    </div>
    {error ? <p className="form-message error">{error}</p> : values.length ? <>
      <svg className="timeseries-chart" viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label={`กราฟ ${config.dataBinding.pointKey}`}>
        <line x1="0" y1="36" x2="100" y2="36" />
        <polyline points={polyline} />
      </svg>
      <div className="timeseries-stats"><span>Min <strong>{format(minimum)}</strong></span><span>Max <strong>{format(maximum)}</strong></span><span>Latest <strong>{format(values[values.length - 1].value)}</strong></span></div>
    </> : <div className="table-state">ไม่มีค่า {config.dataBinding.pointKey} ในช่วงเวลานี้</div>}
  </div>;
}

function TimeseriesConfigEditor({ initial, onClose, onSave }: { initial?: TimeseriesWidgetConfig; onClose: () => void; onSave: (config: TimeseriesWidgetConfig) => void }) {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [latest, setLatest] = useState<LatestTelemetry[]>([]);
  const [plantId, setPlantId] = useState(initial?.dataBinding.plantId ?? "");
  const [deviceId, setDeviceId] = useState(initial?.dataBinding.deviceId ?? "");
  const [pointKey, setPointKey] = useState(initial?.dataBinding.pointKey ?? "");
  const [timeRangeHours, setTimeRangeHours] = useState<1 | 6 | 24 | 168>(initial?.dataBinding.timeRangeHours ?? 24);
  const [unit, setUnit] = useState(initial?.display.unit ?? "");
  const [decimals, setDecimals] = useState(initial?.display.decimals ?? 1);
  const [error, setError] = useState("");

  useEffect(() => {
    void api("/api/v1/plants").then(async (response) => {
      if (!response.ok) throw new Error("ไม่สามารถโหลด Plant ได้");
      setPlants((await response.json()) as Plant[]);
    }).catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "ไม่สามารถโหลด Plant ได้"));
  }, []);

  useEffect(() => {
    if (!plantId) {
      setDevices([]);
      setLatest([]);
      return;
    }
    const controller = new AbortController();
    void Promise.all([
      api(`/api/v1/plants/${encodeURIComponent(plantId)}/devices`, { signal: controller.signal }),
      api(`/api/v1/plants/${encodeURIComponent(plantId)}/telemetry/latest`, { signal: controller.signal }),
    ]).then(async ([deviceResponse, latestResponse]) => {
      if (!deviceResponse.ok || !latestResponse.ok) throw new Error("ไม่สามารถโหลด Device หรือ point ได้");
      setDevices((await deviceResponse.json()) as Device[]);
      setLatest((await latestResponse.json()) as LatestTelemetry[]);
      setError("");
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "ไม่สามารถโหลด Device หรือ point ได้");
    });
    return () => controller.abort();
  }, [plantId]);

  const pointOptions = Object.keys(latest.find((reading) => reading.deviceId === deviceId)?.dataItemMap ?? {}).sort();

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!plantId || !deviceId || !pointKey.trim() || decimals < 0 || decimals > 6) {
      setError("กรุณาระบุ Plant, Device, point key และ decimals ให้ครบ");
      return;
    }
    onSave({ version: 1, dataBinding: { plantId, deviceId, pointKey: pointKey.trim(), timeRangeHours }, display: { unit: unit.trim(), decimals } });
  }

  return <div className="modal-backdrop" role="presentation">
    <section className="plant-editor chart-editor" role="dialog" aria-modal="true" aria-label="ตั้งค่ากราฟ">
      <header><div><p>Timeseries widget</p><h2>ตั้งค่ากราฟ</h2></div><button className="icon-button" onClick={onClose} title="ปิด" aria-label="ปิด"><X size={18} /></button></header>
      <form onSubmit={submit}>
        <label>Plant<select value={plantId} onChange={(event) => { setPlantId(event.target.value); setDeviceId(""); setPointKey(""); }} required><option value="">เลือก Plant</option>{plants.map((plant) => <option key={plant.id} value={plant.id}>{plant.name} ({plant.code})</option>)}</select></label>
        <label>Device<select value={deviceId} onChange={(event) => { setDeviceId(event.target.value); setPointKey(""); }} required disabled={!plantId}><option value="">เลือก Device</option>{devices.map((device) => <option key={device.id} value={device.id}>{device.name} ({device.externalId})</option>)}</select></label>
        <label className="full-field">Point key<input list="timeseries-point-keys" value={pointKey} onChange={(event) => setPointKey(event.target.value)} maxLength={200} required /><datalist id="timeseries-point-keys">{pointOptions.map((key) => <option key={key} value={key} />)}</datalist></label>
        <label>ช่วงเวลา<select value={timeRangeHours} onChange={(event) => setTimeRangeHours(Number(event.target.value) as 1 | 6 | 24 | 168)}><option value={1}>1 ชั่วโมง</option><option value={6}>6 ชั่วโมง</option><option value={24}>24 ชั่วโมง</option><option value={168}>7 วัน</option></select></label>
        <label>Unit<input value={unit} onChange={(event) => setUnit(event.target.value)} maxLength={20} placeholder="kW" /></label>
        <label>Decimals<input type="number" min="0" max="6" value={decimals} onChange={(event) => setDecimals(Number(event.target.value))} required /></label>
        {error && <p className="form-message error full-field">{error}</p>}
        <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose}>ยกเลิก</button><button className="primary-button"><Save size={17} /> บันทึกการตั้งค่า</button></div>
      </form>
    </section>
  </div>;
}

function normalizeDashboardLayouts(layouts: ResponsiveLayouts<"lg" | "md" | "sm"> | DashboardLayouts): DashboardLayouts {
  const normalize = (items: readonly DashboardLayoutItem[] | undefined) => (items ?? []).map(({ i, x, y, w, h }) => ({ i, x, y, w, h }));
  return { lg: normalize(layouts.lg), md: normalize(layouts.md), sm: normalize(layouts.sm) };
}

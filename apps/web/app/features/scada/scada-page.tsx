"use client";

import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MiniMap,
  NodeResizer,
  Position,
  ReactFlow,
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type NodeProps,
  type NodeTypes,
  type Viewport,
} from "@xyflow/react";
import {
  Activity,
  ArrowLeft,
  BellRing,
  CircleGauge,
  Clock3,
  Eye,
  FilePlus2,
  Fullscreen,
  Grid2X2,
  History,
  ImageIcon,
  LoaderCircle,
  Pencil,
  Radio,
  RefreshCw,
  RotateCcw,
  Rows3,
  Save,
  Send,
  SunMedium,
  Shapes,
  SquareDashed,
  Table2,
  TextQuote,
  Trash2,
  Type as TypeIcon,
  Workflow,
  Zap,
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import { useRealtimeSocket } from "../../lib/realtime";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";
import { Tooltip, TooltipTrigger, TooltipContent } from "../../components/ui/tooltip";
import { Button } from "../../components/ui/button";
import type {
  Device,
  LatestTelemetry,
  Plant,
  PublishedScadaScreen,
  ScadaDesign,
  ScadaDataItem,
  ScadaNodeData,
  ScadaNodeType,
  ScadaScreen,
  ScadaScreenSummary,
  ScadaScreenVersion,
} from "../../lib/types";

type RuntimeScadaNodeData = ScadaNodeData & { latest?: LatestTelemetry; latestByDevice?: Record<string, LatestTelemetry> };
type FlowNode = Node<RuntimeScadaNodeData>;
type FlowEdge = Edge;
type BuilderMode = "edit" | "preview" | "published";

const nodeTypes = {
  equipment: EquipmentNode,
  metric: MetricNode,
  label: LabelNode,
  shape: ShapeNode,
  section: SectionNode,
  led: LedNode,
  clock: ClockNode,
  image: ImageNode,
  table: DataTableNode,
  alarms: AlarmListNode,
  ticker: TextTickerNode,
  "device-summary": DeviceSummaryNode,
} satisfies NodeTypes;

const paletteEntries: Array<{ type: ScadaNodeType; title: string; description: string; icon: typeof Zap; requiresDevice?: boolean }> = [
  { type: "equipment", title: "Equipment", description: "Solar, inverter, meter, grid", icon: Zap },
  { type: "metric", title: "Live value", description: "Text, gauge, progress, tank", icon: CircleGauge, requiresDevice: true },
  { type: "led", title: "LED status", description: "สถานะ on/off จากค่าล่าสุด", icon: Radio, requiresDevice: true },
  { type: "table", title: "Data table", description: "แสดงค่าหลาย point", icon: Table2, requiresDevice: true },
  { type: "alarms", title: "Alarm list", description: "ตรวจ threshold หลาย point", icon: BellRing, requiresDevice: true },
  { type: "device-summary", title: "Device parameters", description: "แสดงทุก parameter ของ Device ที่เลือก", icon: Rows3, requiresDevice: true },
  { type: "label", title: "Label", description: "หัวข้อและคำอธิบาย", icon: TypeIcon },
  { type: "ticker", title: "Text ticker", description: "ข้อความประกาศบนจอ", icon: TextQuote },
  { type: "shape", title: "Shape", description: "Rectangle, circle, diamond", icon: Shapes },
  { type: "section", title: "Section", description: "กรอบจัดกลุ่มอุปกรณ์", icon: SquareDashed },
  { type: "image", title: "Image", description: "รูปจาก HTTPS หรือ managed path", icon: ImageIcon },
  { type: "clock", title: "Plant clock", description: "เวลาและ timezone ที่ระบุ", icon: Clock3 },
];

export function ScadaPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [screens, setScreens] = useState<ScadaScreenSummary[]>([]);
  const [screen, setScreen] = useState<ScadaScreen | null>(null);
  const [versions, setVersions] = useState<ScadaScreenVersion[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [latestByDevice, setLatestByDevice] = useState<Record<string, LatestTelemetry>>({});
  const [published, setPublished] = useState<PublishedScadaScreen | null>(null);
  const [draftDesign, setDraftDesign] = useState<ScadaDesign | null>(null);
  const [draftName, setDraftName] = useState("");
  const [mode, setMode] = useState<BuilderMode>("edit");
  const [saveState, setSaveState] = useState<"saved" | "dirty" | "saving" | "conflict" | "error">("saved");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [createPlantID, setCreatePlantID] = useState("");
  const [createName, setCreateName] = useState("");
  const [canvasEpoch, setCanvasEpoch] = useState(0);
  const revisionRef = useRef(0);
  const savingRef = useRef(false);
  const [retryTick, setRetryTick] = useState(0);

  const loadLibrary = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [plantResponse, screenResponse] = await Promise.all([api("/api/v1/plants"), api("/api/v1/scada/screens")]);
      if (!plantResponse.ok) throw new Error("ไม่สามารถโหลด Plant สำหรับ SCADA ได้");
      if (screenResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู SCADA Screen");
      if (!screenResponse.ok) throw new Error("ไม่สามารถโหลด SCADA Screen ได้");
      const nextPlants = (await plantResponse.json()) as Plant[];
      setPlants(nextPlants);
      setCreatePlantID((current) => current || nextPlants[0]?.id || "");
      setScreens((await screenResponse.json()) as ScadaScreenSummary[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadLibrary(); }, [loadLibrary]);

  const loadScreen = useCallback(async (screenID: string) => {
    setLoading(true);
    setError("");
    try {
      const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screenID)}`);
      if (!response.ok) throw new Error(response.status === 404 ? "ไม่พบ SCADA Screen หรืออยู่นอกขอบเขตสิทธิ์" : "ไม่สามารถโหลด SCADA Screen ได้");
      const next = (await response.json()) as ScadaScreen;
      const [deviceResponse, telemetryResponse, versionResponse] = await Promise.all([
        api(`/api/v1/plants/${encodeURIComponent(next.plantId)}/devices`),
        api(`/api/v1/plants/${encodeURIComponent(next.plantId)}/telemetry/latest`),
        api(`/api/v1/scada/screens/${encodeURIComponent(next.id)}/versions`),
      ]);
      setScreen(next);
      setDraftName(next.name);
      setDraftDesign(next.design);
      setDevices(deviceResponse.ok ? (await deviceResponse.json()) as Device[] : []);
      if (telemetryResponse.ok) {
        const readings = (await telemetryResponse.json()) as LatestTelemetry[];
        setLatestByDevice(Object.fromEntries(readings.map((reading) => [reading.deviceId, reading])));
      } else setLatestByDevice({});
      setVersions(versionResponse.ok ? (await versionResponse.json()) as ScadaScreenVersion[] : []);
      if (next.canEdit) setPublished(null);
      else {
        const publishedResponse = await api(`/api/v1/scada/screens/${encodeURIComponent(next.id)}/published`);
        setPublished(publishedResponse.ok ? (await publishedResponse.json()) as PublishedScadaScreen : null);
      }
      setMode(next.canEdit ? "edit" : "published");
      setSaveState("saved");
      revisionRef.current = 0;
      setCanvasEpoch((value) => value + 1);
      setScreens((current) => current.map((item) => item.id === next.id ? next : item));
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useRealtimeSocket(screen?.plantId, (message) => {
    if (message.type === "telemetry.snapshot") {
      setLatestByDevice(Object.fromEntries(message.data.map((reading) => [reading.deviceId, reading])));
    }
  }, Boolean(screen));

  useEffect(() => {
    if (!screen || !draftDesign || !screen.canEdit || saveState !== "dirty") return;
    const revision = revisionRef.current;
    const timer = setTimeout(() => void persistDraft(revision), 1200);
    return () => clearTimeout(timer);
    // retryTick intentionally schedules the newest edit after an in-flight save.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draftDesign, draftName, retryTick, saveState, screen]);

  async function persistDraft(revision: number) {
    if (!screen || !draftDesign || savingRef.current) return;
    savingRef.current = true;
    setSaveState("saving");
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screen.id)}`, {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ draftVersion: screen.draftVersion, name: draftName, design: draftDesign }),
    });
    if (response.ok) {
      const next = (await response.json()) as ScadaScreen;
      setScreen(next);
      setScreens((current) => current.map((item) => item.id === next.id ? next : item));
      if (revisionRef.current === revision) setSaveState("saved");
      else {
        setSaveState("dirty");
        setRetryTick((value) => value + 1);
      }
    } else if (response.status === 409) setSaveState("conflict");
    else {
      setSaveState("error");
      setError("ไม่สามารถบันทึก Draft ได้ กรุณาตรวจ binding และลองอีกครั้ง");
    }
    savingRef.current = false;
  }

  function markDesign(next: ScadaDesign) {
    setDraftDesign(next);
    revisionRef.current += 1;
    setSaveState("dirty");
  }

  function markName(value: string) {
    setDraftName(value);
    revisionRef.current += 1;
    setSaveState("dirty");
  }

  async function createScreen(event: FormEvent) {
    event.preventDefault();
    setError("");
    const response = await api("/api/v1/scada/screens", {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ plantId: createPlantID, name: createName }),
    });
    if (!response.ok) {
      setError(response.status === 409 ? "ชื่อ Screen นี้มีอยู่แล้วใน Plant" : response.status === 403 || response.status === 404 ? "บัญชีนี้ไม่มีสิทธิ์สร้าง Screen ใน Plant นี้" : "ไม่สามารถสร้าง SCADA Screen ได้");
      return;
    }
    const next = (await response.json()) as ScadaScreen;
    setCreateOpen(false);
    setCreateName("");
    setScreens((current) => [next, ...current]);
    await loadScreen(next.id);
  }

  async function showPublished() {
    if (!screen) return;
    setMode("published");
    setError("");
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screen.id)}/published`);
    if (response.ok) setPublished((await response.json()) as PublishedScadaScreen);
    else {
      setPublished(null);
      setError("Screen นี้ยังไม่มี Published version");
    }
  }

  async function publishScreen() {
    if (!screen || saveState !== "saved") return;
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screen.id)}/publish`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ draftVersion: screen.draftVersion, publishedVersion: screen.publishedVersion }),
    });
    if (!response.ok) {
      setError(response.status === 409 ? "Draft หรือ Published version เปลี่ยนแล้ว กรุณาโหลดใหม่" : "ไม่สามารถ Publish Screen ได้");
      return;
    }
    const result = (await response.json()) as PublishedScadaScreen;
    setScreen({ ...screen, publishedVersion: result.publishedVersion });
    setPublished(result);
    setMode("published");
    await reloadVersions(screen.id);
  }

  async function reloadVersions(screenID: string) {
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screenID)}/versions`);
    if (response.ok) setVersions((await response.json()) as ScadaScreenVersion[]);
  }

  async function rollback(version: ScadaScreenVersion) {
    if (!screen || !window.confirm(`Rollback Published Viewer ไป Version ${version.version}? Draft ปัจจุบันจะไม่ถูกแก้ไข`)) return;
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screen.id)}/rollback`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ targetVersion: version.version, publishedVersion: screen.publishedVersion }),
    });
    if (!response.ok) {
      setError(response.status === 409 ? "Published version เปลี่ยนแล้ว กรุณาโหลดใหม่" : "ไม่สามารถ Rollback ได้");
      return;
    }
    const result = (await response.json()) as PublishedScadaScreen;
    setScreen({ ...screen, publishedVersion: result.publishedVersion });
    setPublished(result);
    setMode("published");
    await reloadVersions(screen.id);
  }

  async function hardDelete() {
    if (!screen) return;
    const expected = "DELETE";
    if (window.prompt(`คำสั่งนี้จะลบ Draft และ Published history ทั้งหมดถาวร\nพิมพ์ ${expected}`) !== expected) return;
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screen.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (!response.ok) {
      setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ลบ Screen ถาวรได้" : "ไม่สามารถลบ Screen ได้");
      return;
    }
    setScreen(null);
    setDraftDesign(null);
    await loadLibrary();
  }

  if (!screen || !draftDesign) {
    return <ScadaLibrary plants={plants} screens={screens} loading={loading} error={error} createOpen={createOpen} createPlantID={createPlantID} createName={createName} onRefresh={loadLibrary} onOpen={loadScreen} onCreateOpen={setCreateOpen} onCreatePlantID={setCreatePlantID} onCreateName={setCreateName} onCreate={createScreen} />;
  }

  const activeDesign = mode === "published" ? published?.design : draftDesign;
  const editable = mode === "edit" && screen.canEdit;
  return <div className="content scada-content">
    <div className="scada-titlebar">
      <div className="registry-title">
        <Button variant="icon" onClick={() => { setScreen(null); setDraftDesign(null); void loadLibrary(); }} title="กลับไปรายการ Screen" aria-label="กลับไปรายการ Screen"><ArrowLeft size={18} /></Button>
        <div><p>{screen.plantCode} · {screen.plantName}</p><input aria-label="ชื่อ SCADA Screen" value={draftName} onChange={(event) => markName(event.target.value)} disabled={!editable} maxLength={100} /></div>
      </div>
      <div className="scada-actions">
        {screen.canEdit && <SaveState state={saveState} />}
        <Tabs value={mode} onValueChange={(value) => { if (value === "published") void showPublished(); else setMode(value as BuilderMode); }}>
          <TabsList aria-label="โหมด SCADA">
            {screen.canEdit && <TabsTrigger value="edit"><Pencil size={15} /> Edit</TabsTrigger>}
            {screen.canEdit && <TabsTrigger value="preview"><Eye size={15} /> Preview</TabsTrigger>}
            <TabsTrigger value="published"><Radio size={15} /> Published</TabsTrigger>
          </TabsList>
        </Tabs>
        {screen.canPublish && <Button compact disabled={saveState !== "saved"} onClick={() => void publishScreen()}><Send size={16} /> Publish</Button>}
        {screen.canHardDelete && <Button variant="icon" danger onClick={() => void hardDelete()} title="ลบ Screen ถาวร" aria-label="ลบ Screen ถาวร"><Trash2 size={17} /></Button>}
      </div>
    </div>
    {error && <p className="form-message error">{error}</p>}
    {saveState === "conflict" && <div className="scada-conflict"><strong>Draft มีการแก้ไขจากที่อื่น</strong><span>โหลดเวอร์ชันล่าสุดก่อนแก้ต่อเพื่อป้องกันข้อมูลหาย</span><Button variant="secondary" compact onClick={() => void loadScreen(screen.id)}><RefreshCw size={16} /> โหลดใหม่</Button></div>}
    {activeDesign ? <ScadaCanvas key={`${screen.id}-${canvasEpoch}-${mode}`} design={activeDesign} editable={editable} devices={devices} latestByDevice={latestByDevice} versions={versions} canPublish={screen.canPublish} onDesignChange={markDesign} onRollback={rollback} /> : <div className="table-state">ยังไม่มี Published version</div>}
  </div>;
}

function ScadaLibrary({ plants, screens, loading, error, createOpen, createPlantID, createName, onRefresh, onOpen, onCreateOpen, onCreatePlantID, onCreateName, onCreate }: {
  plants: Plant[]; screens: ScadaScreenSummary[]; loading: boolean; error: string; createOpen: boolean; createPlantID: string; createName: string;
  onRefresh: () => Promise<void>; onOpen: (id: string) => Promise<void>; onCreateOpen: (open: boolean) => void; onCreatePlantID: (value: string) => void; onCreateName: (value: string) => void; onCreate: (event: FormEvent) => Promise<void>;
}) {
  return <div className="content scada-content">
    <div className="section-heading"><div><p>Fixed-canvas operational screens</p><h2>SCADA Screens</h2></div><div className="heading-actions"><Button variant="icon" onClick={() => void onRefresh()} title="รีเฟรช" aria-label="รีเฟรช SCADA Screens"><RefreshCw size={18} /></Button><Button compact onClick={() => onCreateOpen(true)}><FilePlus2 size={17} /> สร้าง Screen</Button></div></div>
    {error && <p className="form-message error">{error}</p>}
    <section className="scada-library" aria-label="SCADA Screens">
      <div className="scada-library-head"><span>Screen</span><span>Plant</span><span>Draft</span><span>Published</span><span>Updated</span><span aria-label="คำสั่ง" /></div>
      {screens.map((item) => <button key={item.id} className="scada-library-row" onClick={() => void onOpen(item.id)}>
        <div><Workflow size={18} /><span><strong>{item.name}</strong><small>{item.id}</small></span></div><div><strong>{item.plantName}</strong><small>{item.plantCode}</small></div><span>v{item.draftVersion}</span><span className={item.publishedVersion > 0 ? "status active" : "status revoked"}>{item.publishedVersion > 0 ? `v${item.publishedVersion}` : "ยังไม่ Publish"}</span><time>{formatDate(item.updatedAt)}</time><Pencil size={17} />
      </button>)}
      {loading && <div className="table-state">กำลังโหลด SCADA Screens</div>}
      {!loading && !error && screens.length === 0 && <div className="table-state">ยังไม่มี SCADA Screen — สร้าง Screen แรกสำหรับ Plant ที่ต้องการ</div>}
    </section>
    <Dialog open={createOpen} onOpenChange={onCreateOpen}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div><DialogDescription>Phase 3B</DialogDescription><DialogTitle>สร้าง SCADA Screen</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={(event) => void onCreate(event)}>
            <label className="full-field">Plant
              <Select value={createPlantID} onValueChange={onCreatePlantID}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>{plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.code} · {plant.name}</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label className="full-field">ชื่อ Screen<input autoFocus value={createName} onChange={(event) => onCreateName(event.target.value)} maxLength={100} placeholder="Single Line Diagram" required /></label>
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={() => onCreateOpen(false)}>ยกเลิก</Button><Button><FilePlus2 size={17} /> สร้าง Draft</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  </div>;
}

export function ScadaCanvas({ design, editable, devices, latestByDevice, versions, canPublish, onDesignChange, onRollback, hideInspector, showMinimap = true, locked = false }: {
  design: ScadaDesign; editable: boolean; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; versions: ScadaScreenVersion[]; canPublish: boolean; onDesignChange: (design: ScadaDesign) => void; onRollback: (version: ScadaScreenVersion) => Promise<void>; hideInspector?: boolean; showMinimap?: boolean; locked?: boolean;
}) {
  const [nodes, setNodes] = useState<FlowNode[]>(() => design.nodes.map((node) => ({ ...node, zIndex: node.type === "section" ? -1 : 0, style: node.width && node.height ? { width: node.width, height: node.height } : undefined })));
  const [edges, setEdges] = useState<FlowEdge[]>(() => design.edges.map((edge) => ({ ...edge, animated: false })));
  const [viewport, setViewport] = useState<Viewport>(design.viewport);
  const [selectedID, setSelectedID] = useState("");
  const stageRef = useRef<HTMLDivElement>(null);
  const selected = nodes.find((node) => node.id === selectedID);

  function emit(nextNodes: FlowNode[], nextEdges: FlowEdge[], nextViewport = viewport) {
    onDesignChange({
      version: 1,
      nodes: nextNodes.map((node) => {
        const { latest: _latest, latestByDevice: _latestByDevice, ...data } = node.data;
        const width = node.measured?.width ?? node.width;
        const height = node.measured?.height ?? node.height;
        return { id: node.id, type: node.type as ScadaNodeType, position: node.position, data, ...(width ? { width } : {}), ...(height ? { height } : {}) };
      }),
      edges: nextEdges.map((edge) => ({ id: edge.id, source: edge.source, target: edge.target, type: edge.type === "default" ? "default" : "smoothstep" })),
      viewport: nextViewport,
    });
  }

  function nodesChanged(changes: NodeChange<FlowNode>[]) {
    if (!editable) return;
    const next = applyNodeChanges(changes, nodes);
    setNodes(next);
    emit(next, edges);
  }

  function edgesChanged(changes: EdgeChange<FlowEdge>[]) {
    if (!editable) return;
    const next = applyEdgeChanges(changes, edges);
    setEdges(next);
    emit(nodes, next);
  }

  function connect(connection: Connection) {
    if (!editable || !connection.source || !connection.target || connection.source === connection.target) return;
    if (edges.some((edge) => edge.source === connection.source && edge.target === connection.target)) return;
    const next = addEdge({ ...connection, id: `edge-${crypto.randomUUID()}`, type: "smoothstep", animated: false }, edges);
    setEdges(next);
    emit(nodes, next);
  }

  function addNode(type: ScadaNodeType) {
    if (!editable) return;
    if (paletteEntries.find((entry) => entry.type === type)?.requiresDevice && devices.length === 0) return;
    const id = `${type}-${crypto.randomUUID()}`;
    const firstPointKey = devices[0] ? Object.keys(latestByDevice[devices[0].id]?.dataItemMap || {}).sort()[0] : undefined;
    const binding = devices[0] ? { deviceId: devices[0].id, pointKey: firstPointKey || "", unit: "", decimals: 1 } : undefined;
    const defaults: Record<ScadaNodeType, ScadaNodeData> = {
      equipment: { label: "Inverter", equipmentKind: "inverter" },
      metric: { label: "Active power", binding, displayType: "text", minValue: 0, maxValue: 100 },
      label: { label: "Section label" },
      shape: { label: "Shape", shapeKind: "rectangle" },
      section: { label: "Equipment group" },
      led: { label: "Running", binding, onValue: 1 },
      clock: { label: "Plant time", timezone: "Asia/Bangkok" },
      image: { label: "Reference image", imageUrl: "" },
      table: { label: "Measurements", items: [{ label: "Active power", binding: binding! }] },
      alarms: { label: "Threshold alarms", items: [{ label: "Active power", binding: binding!, minAlarm: 0, maxAlarm: 100 }] },
      ticker: { label: "Message", text: "Plant operating normally" },
      "device-summary": { label: "Device parameters", deviceId: devices[0]?.id },
    };
    const dimensions: { width?: number; height?: number } = type === "section" ? { width: 360, height: 220 } : type === "image" ? { width: 260, height: 160 } : type === "device-summary" ? { width: 300, height: 240 } : type === "table" || type === "alarms" ? { width: 280, height: 180 } : type === "shape" ? { width: 140, height: 90 } : {};
    const nextNode: FlowNode = { id, type, position: { x: 100 + nodes.length * 28, y: 100 + nodes.length * 24 }, data: defaults[type], zIndex: type === "section" ? -1 : 0, ...dimensions, ...(dimensions.width && dimensions.height ? { style: dimensions } : {}) };
    const next = [...nodes, nextNode];
    setNodes(next);
    setSelectedID(id);
    emit(next, edges);
  }

  function updateSelected(data: ScadaNodeData) {
    const next = nodes.map((node) => node.id === selectedID ? { ...node, data } : node);
    setNodes(next);
    emit(next, edges);
  }

  function removeSelected() {
    const nextNodes = nodes.filter((node) => node.id !== selectedID);
    const nextEdges = edges.filter((edge) => edge.source !== selectedID && edge.target !== selectedID);
    setNodes(nextNodes); setEdges(nextEdges); setSelectedID(""); emit(nextNodes, nextEdges);
  }

  return <div className={editable ? "scada-workbench editing scada-dark" : hideInspector ? "scada-workbench solo scada-dark" : "scada-workbench viewing scada-dark"}>
    {editable && <aside className="scada-palette"><header><Grid2X2 size={17} /><div><strong>Node palette</strong><small>เพิ่มองค์ประกอบลง fixed canvas</small></div></header>{paletteEntries.map(({ type, title, description, icon: Icon, requiresDevice }) => <button key={type} onClick={() => addNode(type)} disabled={requiresDevice && devices.length === 0} title={requiresDevice && devices.length === 0 ? "Plant นี้ยังไม่มี Device" : undefined}><Icon size={18} /><span><strong>{title}</strong><small>{description}</small></span></button>)}<div className="palette-note"><Workflow size={16} /><p>ใช้ Edge ของ React Flow สำหรับเส้นและลูกศรระหว่าง Node</p></div></aside>}
    <section className="scada-stage-shell" ref={stageRef}>
      <div className="scada-stage-toolbar"><span>{editable ? "Draft canvas · Snap 20px" : "Operational viewer · Read only"}</span><Button variant="icon" onClick={() => void stageRef.current?.requestFullscreen()} title="เต็มจอ" aria-label="แสดง SCADA เต็มจอ"><Fullscreen size={17} /></Button></div>
      <div className="scada-stage">
        <ReactFlow<FlowNode, FlowEdge> nodes={nodes.map((node) => ({ ...node, data: { ...node.data, latest: latestByDevice[node.data.binding?.deviceId || ""], latestByDevice } }))} edges={edges} nodeTypes={nodeTypes} onNodesChange={nodesChanged} onEdgesChange={edgesChanged} onConnect={connect} onNodeClick={(_, node) => setSelectedID(node.id)} onPaneClick={() => setSelectedID("")} onMoveEnd={(_, next) => { setViewport(next); if (editable) emit(nodes, edges, next); }} defaultViewport={viewport} nodesDraggable={editable} nodesConnectable={editable} elementsSelectable={editable} deleteKeyCode={editable ? ["Backspace", "Delete"] : null} panOnDrag={!locked} panOnScroll={!locked} zoomOnScroll={!locked} zoomOnPinch={!locked} zoomOnDoubleClick={!locked} snapToGrid snapGrid={[20, 20]} fitView minZoom={0.1} maxZoom={4} attributionPosition="bottom-left">
          <Background variant={BackgroundVariant.Lines} gap={20} size={1} color="var(--line)" />
          {showMinimap && <MiniMap pannable zoomable nodeColor={(node) => node.type === "metric" ? "var(--action)" : node.type === "equipment" ? "var(--warning)" : "var(--muted)"} />}
          {!locked && <Controls showInteractive={false} />}
        </ReactFlow>
      </div>
    </section>
    {editable && <ScadaInspector selected={selected} devices={devices} latestByDevice={latestByDevice} versions={versions} canPublish={canPublish} onUpdate={updateSelected} onRemove={removeSelected} onRollback={onRollback} />}
    {!editable && !hideInspector && <aside className="scada-inspector viewer-info"><header><History size={17} /><div><strong>Published history</strong><small>เวอร์ชัน immutable</small></div></header><VersionHistory versions={versions} canPublish={canPublish} onRollback={onRollback} /></aside>}
  </div>;
}

function VersionHistory({ versions, canPublish, onRollback }: { versions: ScadaScreenVersion[]; canPublish: boolean; onRollback: (version: ScadaScreenVersion) => Promise<void> }) {
  return <div className="version-list">{versions.map((version) => <div key={version.version}><span><strong>Version {version.version}</strong><small><Clock3 size={12} /> {formatDate(version.publishedAt)} · Draft {version.sourceDraftVersion}</small></span>{version.current ? <span className="status active">Current</span> : canPublish ? <Button variant="icon" onClick={() => void onRollback(version)} title={`Rollback ไป Version ${version.version}`} aria-label={`Rollback ไป Version ${version.version}`}><RotateCcw size={15} /></Button> : null}</div>)}{versions.length === 0 && <p className="muted-text">ยังไม่มี Published version</p>}</div>;
}

function SaveState({ state }: { state: "saved" | "dirty" | "saving" | "conflict" | "error" }) {
  const labels = { saved: "Saved", dirty: "Unsaved", saving: "Saving", conflict: "Conflict", error: "Save failed" };
  return <span className={`save-state ${state}`}>{state === "saving" ? <LoaderCircle className="spin" size={14} /> : state === "saved" ? <Save size={14} /> : <Activity size={14} />}{labels[state]}</span>;
}

function ScadaInspector({ selected, devices, latestByDevice, versions, canPublish, onUpdate, onRemove, onRollback }: {
  selected?: FlowNode; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; versions: ScadaScreenVersion[]; canPublish: boolean;
  onUpdate: (data: ScadaNodeData) => void; onRemove: () => void; onRollback: (version: ScadaScreenVersion) => Promise<void>;
}) {
  if (!selected) return <aside className="scada-inspector"><header><History size={17} /><div><strong>Published history</strong><small>เลือก Node เพื่อแก้ไขคุณสมบัติ</small></div></header><VersionHistory versions={versions} canPublish={canPublish} onRollback={onRollback} /></aside>;
  const data = selected.data;
  const update = (patch: Partial<ScadaNodeData>) => onUpdate({ ...data, ...patch });
  const bound = selected.type === "metric" || selected.type === "led";
  const listed = selected.type === "table" || selected.type === "alarms";
  const deviceOnly = selected.type === "device-summary";
  return <aside className="scada-inspector">
    <header><Pencil size={17} /><div><strong>Node properties</strong><Tooltip><TooltipTrigger asChild><small className="cursor-help underline decoration-dotted">{selected.type} · {selected.id.slice(0, 12)}</small></TooltipTrigger><TooltipContent>Node type: {selected.type}<br />Full ID: {selected.id}</TooltipContent></Tooltip></div></header>
    <label>Label<input value={data.label} maxLength={100} onChange={(event) => update({ label: event.target.value })} /></label>
    {selected.type === "equipment" && <label>Equipment type<select value={data.equipmentKind || "inverter"} onChange={(event) => update({ equipmentKind: event.target.value as ScadaNodeData["equipmentKind"] })}><option value="solar-panel">Solar panel</option><option value="inverter">Inverter</option><option value="meter">Meter</option><option value="grid">Grid</option></select></label>}
    {bound && <BindingEditor binding={data.binding} devices={devices} latestByDevice={latestByDevice} onChange={(binding) => update({ binding })} />}
    {deviceOnly && <label>Device<select value={data.deviceId || ""} onChange={(event) => update({ deviceId: event.target.value })}>{devices.map((device) => <option key={device.id} value={device.id}>{device.name}</option>)}</select></label>}
    {selected.type === "metric" && <><label>Display<select value={data.displayType || "text"} onChange={(event) => update({ displayType: event.target.value as ScadaNodeData["displayType"] })}><option value="text">Text</option><option value="gauge">Gauge</option><option value="progress">Progress</option><option value="tank">Tank</option></select></label><div className="inspector-grid"><NumberField label="Minimum" value={data.minValue} onChange={(minValue) => update({ minValue })} /><NumberField label="Maximum" value={data.maxValue} onChange={(maxValue) => update({ maxValue })} /></div></>}
    {selected.type === "led" && <NumberField label="On when value equals" value={data.onValue} onChange={(onValue) => update({ onValue })} />}
    {selected.type === "shape" && <label>Shape<select value={data.shapeKind || "rectangle"} onChange={(event) => update({ shapeKind: event.target.value as ScadaNodeData["shapeKind"] })}><option value="rectangle">Rectangle</option><option value="circle">Circle</option><option value="triangle">Triangle</option><option value="diamond">Diamond</option><option value="hexagon">Hexagon</option></select></label>}
    {selected.type === "clock" && <label>Timezone<select value={data.timezone || "Asia/Bangkok"} onChange={(event) => update({ timezone: event.target.value })}><option value="Asia/Bangkok">Asia/Bangkok</option><option value="UTC">UTC</option><option value="Asia/Singapore">Asia/Singapore</option><option value="Asia/Tokyo">Asia/Tokyo</option></select></label>}
    {selected.type === "image" && <label>Image URL<input value={data.imageUrl || ""} maxLength={2048} placeholder="https://… or /images/…" onChange={(event) => update({ imageUrl: event.target.value })} /></label>}
    {selected.type === "ticker" && <label>Message<input value={data.text || ""} maxLength={200} onChange={(event) => update({ text: event.target.value })} /></label>}
    {listed && <DataItemsEditor items={data.items || []} devices={devices} latestByDevice={latestByDevice} alarms={selected.type === "alarms"} onChange={(items) => update({ items })} />}
    <Button variant="secondary" danger onClick={onRemove}><Trash2 size={16} /> ลบ Node</Button>
  </aside>;
}

function NumberField({ label, value, onChange }: { label: string; value?: number; onChange: (value?: number) => void }) {
  return <label>{label}<input type="number" value={value ?? ""} onChange={(event) => onChange(event.target.value === "" ? undefined : Number(event.target.value))} /></label>;
}

function BindingEditor({ binding, devices, latestByDevice, onChange }: { binding?: ScadaNodeData["binding"]; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; onChange: (binding: NonNullable<ScadaNodeData["binding"]>) => void }) {
  const current = binding || { deviceId: devices[0]?.id || "", pointKey: "", unit: "kW", decimals: 1 };
  const knownKeys = Object.keys(latestByDevice[current.deviceId]?.dataItemMap || {}).sort();
  const pointOptions = current.pointKey && !knownKeys.includes(current.pointKey) ? [current.pointKey, ...knownKeys] : knownKeys;
  function changeDevice(deviceId: string) {
    const nextKeys = Object.keys(latestByDevice[deviceId]?.dataItemMap || {}).sort();
    onChange({ ...current, deviceId, pointKey: nextKeys[0] || "" });
  }
  return <div className="binding-editor">
    <label>Device<select value={current.deviceId} onChange={(event) => changeDevice(event.target.value)}>{devices.map((device) => <option key={device.id} value={device.id}>{device.name}</option>)}</select></label>
    <label>Parameter<select value={current.pointKey} onChange={(event) => onChange({ ...current, pointKey: event.target.value })}>
      {!current.pointKey && <option value="">{pointOptions.length === 0 ? "ยังไม่มีข้อมูล" : "-- เลือก parameter --"}</option>}
      {pointOptions.map((key) => <option key={key} value={key}>{key}</option>)}
    </select></label>
    <div className="inspector-grid"><label>Unit<input value={current.unit} maxLength={20} onChange={(event) => onChange({ ...current, unit: event.target.value })} /></label><label>Decimals<input type="number" min={0} max={6} value={current.decimals} onChange={(event) => onChange({ ...current, decimals: Number(event.target.value) })} /></label></div>
  </div>;
}

function DataItemsEditor({ items, devices, latestByDevice, alarms, onChange }: { items: ScadaDataItem[]; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; alarms: boolean; onChange: (items: ScadaDataItem[]) => void }) {
  const fallbackDeviceId = devices[0]?.id || "";
  const fallbackPointKey = Object.keys(latestByDevice[fallbackDeviceId]?.dataItemMap || {}).sort()[0] || "";
  const fallback = { deviceId: fallbackDeviceId, pointKey: fallbackPointKey, unit: "kW", decimals: 1 };
  const patch = (index: number, next: Partial<ScadaDataItem>) => onChange(items.map((item, itemIndex) => itemIndex === index ? { ...item, ...next } : item));
  return <section className="scada-item-editor"><div className="item-editor-heading"><strong>{alarms ? "Alarm points" : "Table rows"}</strong><Button type="button" variant="icon" disabled={items.length >= 20 || devices.length === 0} onClick={() => onChange([...items, { label: `Point ${items.length + 1}`, binding: fallback }])} title="เพิ่ม point" aria-label="เพิ่ม point">+</Button></div>{items.map((item, index) => <div className="scada-item-row" key={`${index}-${item.binding.deviceId}`}><label>Label<input value={item.label} maxLength={100} onChange={(event) => patch(index, { label: event.target.value })} /></label><BindingEditor binding={item.binding} devices={devices} latestByDevice={latestByDevice} onChange={(binding) => patch(index, { binding })} />{alarms && <div className="inspector-grid"><NumberField label="Low alarm" value={item.minAlarm} onChange={(minAlarm) => patch(index, { minAlarm })} /><NumberField label="High alarm" value={item.maxAlarm} onChange={(maxAlarm) => patch(index, { maxAlarm })} /></div>}<Button type="button" variant="text" danger disabled={items.length <= 1} onClick={() => onChange(items.filter((_, itemIndex) => itemIndex !== index))}>ลบแถว</Button></div>)}</section>;
}

function EquipmentNode({ data, selected }: NodeProps<FlowNode>) {
  const icons = { "solar-panel": SunMedium, inverter: Zap, meter: CircleGauge, grid: Workflow };
  const Icon = icons[data.equipmentKind || "inverter"];
  return <div className={selected ? "scada-node equipment selected" : "scada-node equipment"}><NodeHandles /><Icon size={23} /><strong>{data.label}</strong><small>{data.equipmentKind}</small></div>;
}

function MetricNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = readBinding(data, data.binding);
  const min = data.minValue ?? 0; const max = data.maxValue ?? 100; const value = reading.value ?? min;
  return <div className={selected ? "scada-node metric selected" : "scada-node metric"}><NodeHandles /><small>{data.label}</small>{data.displayType === "gauge" && <meter min={min} max={max} value={value} />}{data.displayType === "progress" && <progress max={Math.max(1, max - min)} value={Math.max(0, value - min)} />}{data.displayType === "tank" && <div className="metric-tank" aria-hidden="true"><span style={{ height: `${Math.max(0, Math.min(100, ((value - min) / Math.max(1, max - min)) * 100))}%` }} /></div>}<strong>{reading.missing ? "—" : reading.formatted} <em>{data.binding?.unit}</em></strong><Quality reading={reading} /></div>;
}

function LabelNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={selected ? "scada-node label selected" : "scada-node label"}><NodeHandles /><strong>{data.label}</strong></div>;
}

function ShapeNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={selected ? "scada-node shape selected" : "scada-node shape"}><NodeResizer minWidth={60} minHeight={40} isVisible={selected} /><NodeHandles /><div className={`shape-body ${data.shapeKind || "rectangle"}`}><strong>{data.label}</strong></div></div>;
}

function SectionNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={selected ? "scada-node section selected" : "scada-node section"}><NodeResizer minWidth={180} minHeight={120} isVisible={selected} /><NodeHandles /><strong>{data.label}</strong></div>;
}

function LedNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = readBinding(data, data.binding); const active = !reading.missing && reading.value === (data.onValue ?? 1);
  return <div className={selected ? "scada-node led selected" : "scada-node led"}><NodeHandles /><span className={active ? "led-lamp active" : "led-lamp"} aria-hidden="true" /><strong>{data.label}</strong><span className={`node-quality ${reading.missing ? "missing" : active ? "good" : "normal"}`}>{reading.missing ? "No data" : active ? "ON" : "OFF"}</span></div>;
}

function ClockNode({ data, selected }: NodeProps<FlowNode>) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => { const timer = window.setInterval(() => setNow(new Date()), 1000); return () => window.clearInterval(timer); }, []);
  let time = "Invalid timezone"; try { time = new Intl.DateTimeFormat("en-GB", { timeZone: data.timezone || "Asia/Bangkok", dateStyle: "medium", timeStyle: "medium" }).format(now); } catch { /* backend validation prevents this after save */ }
  return <div className={selected ? "scada-node clock selected" : "scada-node clock"}><NodeHandles /><Clock3 size={18} /><strong>{data.label}</strong><time>{time}</time><small>{data.timezone || "Asia/Bangkok"}</small></div>;
}

function ImageNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={selected ? "scada-node image selected" : "scada-node image"}><NodeResizer minWidth={100} minHeight={80} isVisible={selected} /><NodeHandles />{data.imageUrl ? <img src={data.imageUrl} alt={data.label} /> : <div className="image-placeholder"><ImageIcon size={24} /><span>No image set</span></div>}<small>{data.label}</small></div>;
}

function DataTableNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={selected ? "scada-node data-table selected" : "scada-node data-table"}><NodeResizer minWidth={220} minHeight={130} isVisible={selected} /><NodeHandles /><strong>{data.label}</strong><table><tbody>{(data.items || []).map((item, index) => { const reading = readBinding(data, item.binding); return <tr key={`${index}-${item.binding.pointKey}`}><th>{item.label}</th><td>{reading.missing ? "—" : reading.formatted} <small>{item.binding.unit}</small></td></tr>; })}</tbody></table></div>;
}

function DeviceSummaryNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = data.latestByDevice?.[data.deviceId || ""];
  const entries = reading ? Object.entries(reading.dataItemMap).sort(([a], [b]) => a.localeCompare(b)) : [];
  return <div className={selected ? "scada-node data-table selected" : "scada-node data-table"}><NodeResizer minWidth={220} minHeight={130} isVisible={selected} /><NodeHandles /><strong>{data.label}</strong><table><tbody>{entries.map(([key, value]) => <tr key={key}><th>{key}</th><td>{Number.isFinite(value) ? value.toLocaleString() : "—"}</td></tr>)}{entries.length === 0 && <tr><td colSpan={2}>No data</td></tr>}</tbody></table></div>;
}

function AlarmListNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={selected ? "scada-node alarm-list selected" : "scada-node alarm-list"}><NodeResizer minWidth={240} minHeight={130} isVisible={selected} /><NodeHandles /><strong><BellRing size={15} /> {data.label}</strong><div className="alarm-items">{(data.items || []).map((item, index) => { const reading = readBinding(data, item.binding); const alarm = !reading.missing && ((item.minAlarm != null && reading.value! < item.minAlarm) || (item.maxAlarm != null && reading.value! > item.maxAlarm)); return <div key={`${index}-${item.binding.pointKey}`}><span>{item.label}<small>{reading.missing ? "—" : `${reading.formatted} ${item.binding.unit}`}</small></span><b className={reading.missing ? "missing" : alarm ? "alarm" : "normal"}>{reading.missing ? "No data" : alarm ? "Alarm" : "Normal"}</b></div>; })}</div></div>;
}

function TextTickerNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = data.binding ? readBinding(data, data.binding) : undefined;
  return <div className={selected ? "scada-node ticker selected" : "scada-node ticker"}><NodeHandles /><TextQuote size={15} /><strong>{data.label}</strong><span>{reading ? (reading.missing ? "No data" : `${reading.formatted} ${data.binding?.unit}`) : data.text || "—"}</span></div>;
}

function NodeHandles() {
  return <><Handle type="target" position={Position.Left} /><Handle type="source" position={Position.Right} /><Handle id="top" type="target" position={Position.Top} /><Handle id="bottom" type="source" position={Position.Bottom} /></>;
}

type BindingReading = { value?: number; formatted: string; missing: boolean; observedAt?: string };
function readBinding(data: RuntimeScadaNodeData, binding?: ScadaNodeData["binding"]): BindingReading {
  const latest = binding ? data.latestByDevice?.[binding.deviceId] || data.latest : undefined;
  const value = binding ? latest?.dataItemMap[binding.pointKey] : undefined;
  const missing = value == null || !Number.isFinite(value);
  return { value, missing, formatted: missing ? "—" : value.toFixed(binding?.decimals ?? 1), observedAt: latest?.observedAt };
}

function Quality({ reading }: { reading: BindingReading }) {
  return <span className={reading.missing ? "node-quality missing" : "node-quality good"}>{reading.missing ? "No data" : `Observed ${reading.observedAt ? formatDate(reading.observedAt) : ""}`}</span>;
}

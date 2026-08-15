"use client";

import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MiniMap,
  NodeResizer,
  Panel,
  Position,
  ReactFlow,
  type ReactFlowInstance,
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  getNodesBounds,
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
  AlignHorizontalJustifyCenter,
  AlignHorizontalJustifyEnd,
  AlignHorizontalJustifyStart,
  AlignVerticalJustifyCenter,
  AlignVerticalJustifyEnd,
  AlignVerticalJustifyStart,
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
  Redo2,
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
  Undo2,
  Workflow,
  Zap,
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import { sameSelection } from "../../lib/selection";
import { useRealtimeSocket } from "../../lib/realtime";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";
import { Tooltip, TooltipTrigger, TooltipContent } from "../../components/ui/tooltip";
import { ProgressBar } from "primereact/progressbar";
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
import { loadRegisterCatalogs, pointMeta, type PointMeta } from "../../lib/telemetry-history";
import { usePlatformSession } from "../../components/platform-shell";
import { can } from "../../lib/permissions";

type RuntimeScadaNodeData = ScadaNodeData & {
  latest?: LatestTelemetry;
  latestByDevice?: Record<string, LatestTelemetry>;
  catalogs?: Record<string, Record<string, PointMeta>>;
  builderMode?: boolean;
  // Canvas-only, stripped in emit() before persisting -- powers double-click-to-edit-in-place.
  editing?: boolean;
  onEditCommit?: (patch: Partial<ScadaNodeData>) => void;
  onEditCancel?: () => void;
};
type FlowNode = Node<RuntimeScadaNodeData>;
type FlowEdge = Edge;
type BuilderMode = "edit" | "preview" | "published";

const nodeTypes = {
  equipment: EquipmentNode,
  metric: MetricNode,
  label: LabelNode,
  shape: ShapeNode,
  section: SectionNode,
  group: GroupNode,
  led: LedNode,
  clock: ClockNode,
  image: ImageNode,
  table: DataTableNode,
  alarms: AlarmListNode,
  ticker: TextTickerNode,
  "device-summary": DeviceSummaryNode,
} satisfies NodeTypes;

const paletteEntries: Array<{ type: ScadaNodeType; title: string; description: string; icon: typeof Zap; requiresDevice?: boolean }> = [
  { type: "metric", title: "Live value", description: "Text, gauge, progress, tank", icon: CircleGauge, requiresDevice: true },
  { type: "led", title: "LED status", description: "สถานะ on/off จากค่าล่าสุด", icon: Radio, requiresDevice: true },
  { type: "table", title: "Data table", description: "แสดงค่าหลาย point", icon: Table2, requiresDevice: true },
  { type: "alarms", title: "Alarm list", description: "ตรวจ threshold หลาย point", icon: BellRing, requiresDevice: true },
  { type: "device-summary", title: "Device parameters", description: "แสดงทุก parameter ของ Device ที่เลือก", icon: Rows3, requiresDevice: true },
  { type: "label", title: "Label", description: "หัวข้อและคำอธิบาย", icon: TypeIcon },
  { type: "ticker", title: "Text ticker", description: "ข้อความประกาศบนจอ", icon: TextQuote },
  { type: "shape", title: "Shape", description: "Rectangle, circle, diamond", icon: Shapes },
  { type: "section", title: "Section", description: "กรอบจัดกลุ่มอุปกรณ์", icon: SquareDashed },
  { type: "image", title: "Image", description: "เลือกไฟล์หรือลากรูปลง canvas", icon: ImageIcon },
  { type: "clock", title: "Plant clock", description: "เวลาและ timezone ที่ระบุ", icon: Clock3 },
];

export function ScadaPage() {
  const { user } = usePlatformSession();
  const canCreateScreen = can(user, "scada_screen", "edit");
  const [plants, setPlants] = useState<Plant[]>([]);
  const [screens, setScreens] = useState<ScadaScreenSummary[]>([]);
  const [screen, setScreen] = useState<ScadaScreen | null>(null);
  const [versions, setVersions] = useState<ScadaScreenVersion[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [latestByDevice, setLatestByDevice] = useState<Record<string, LatestTelemetry>>({});
  const [catalogs, setCatalogs] = useState<Record<string, Record<string, PointMeta>>>({});
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
      const screenDevices = deviceResponse.ok ? (await deviceResponse.json()) as Device[] : [];
      setDevices(screenDevices);
      // Register display names for the inspector's Parameter picker. Best
      // effort and deliberately not awaited into the critical path: without it
      // the picker falls back to raw address keys.
      void loadRegisterCatalogs(next.plantId, screenDevices).then(setCatalogs).catch(() => setCatalogs({}));
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
    return <ScadaLibrary plants={plants} screens={screens} loading={loading} error={error} createOpen={createOpen} createPlantID={createPlantID} createName={createName} onRefresh={loadLibrary} onOpen={loadScreen} onCreateOpen={setCreateOpen} onCreatePlantID={setCreatePlantID} onCreateName={setCreateName} onCreate={createScreen} canCreate={canCreateScreen} />;
  }

  const activeDesign = mode === "published" ? published?.design : draftDesign;
  const editable = mode === "edit" && screen.canEdit;
  return <div className="content scada-content">
    <div className="scada-titlebar">
      <div className="registry-title">
        <Button variant="icon" onClick={() => { setScreen(null); setDraftDesign(null); void loadLibrary(); }} title="กลับไปรายการ Screen" aria-label="กลับไปรายการ Screen"><ArrowLeft size={18} /></Button>
        <div><p>{screen.plantCode} · {screen.plantName}</p><TextInput aria-label="ชื่อ SCADA Screen" value={draftName} onChange={(event) => markName(event.target.value)} disabled={!editable} maxLength={100} /></div>
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
    {error && <FormMessage>{error}</FormMessage>}
    {saveState === "conflict" && <div className="scada-conflict"><strong>Draft มีการแก้ไขจากที่อื่น</strong><span>โหลดเวอร์ชันล่าสุดก่อนแก้ต่อเพื่อป้องกันข้อมูลหาย</span><Button variant="secondary" compact onClick={() => void loadScreen(screen.id)}><RefreshCw size={16} /> โหลดใหม่</Button></div>}
    {activeDesign ? <ScadaCanvas key={`${screen.id}-${canvasEpoch}-${mode}`} screenId={screen.id} design={activeDesign} editable={editable} devices={devices} latestByDevice={latestByDevice} catalogs={catalogs} versions={versions} canPublish={screen.canPublish} onDesignChange={markDesign} onRollback={rollback} /> : <div className="table-state">ยังไม่มี Published version</div>}
  </div>;
}

function ScadaLibrary({ plants, screens, loading, error, createOpen, createPlantID, createName, onRefresh, onOpen, onCreateOpen, onCreatePlantID, onCreateName, onCreate, canCreate }: {
  plants: Plant[]; screens: ScadaScreenSummary[]; loading: boolean; error: string; createOpen: boolean; createPlantID: string; createName: string;
  onRefresh: () => Promise<void>; onOpen: (id: string) => Promise<void>; onCreateOpen: (open: boolean) => void; onCreatePlantID: (value: string) => void; onCreateName: (value: string) => void; onCreate: (event: FormEvent) => Promise<void>; canCreate: boolean;
}) {
  return <div className="content scada-content">
    <div className="section-heading"><div><p>Fixed-canvas operational screens</p><h2>SCADA Screens</h2></div><div className="heading-actions"><Button variant="icon" onClick={() => void onRefresh()} title="รีเฟรช" aria-label="รีเฟรช SCADA Screens"><RefreshCw size={18} /></Button>{canCreate && <Button compact onClick={() => onCreateOpen(true)}><FilePlus2 size={17} /> สร้าง Screen</Button>}</div></div>
    {error && <FormMessage>{error}</FormMessage>}
    <section className="scada-library" aria-label="SCADA Screens">
      <div className="scada-library-head"><span>Screen</span><span>Plant</span><span>Draft</span><span>Published</span><span>Updated</span><span aria-label="คำสั่ง" /></div>
      {screens.map((item) => <Button variant="bare" key={item.id} className="scada-library-row" onClick={() => void onOpen(item.id)}>
        <div><Workflow size={18} /><span><strong>{item.name}</strong><small>{item.id}</small></span></div><div><strong>{item.plantName}</strong><small>{item.plantCode}</small></div><span>v{item.draftVersion}</span><StatusTag tone={item.publishedVersion > 0 ? "active" : "revoked"}>{item.publishedVersion > 0 ? `v${item.publishedVersion}` : "ยังไม่ Publish"}</StatusTag><time>{formatDate(item.updatedAt)}</time><Pencil size={17} />
      </Button>)}
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
            <label className="full-field">ชื่อ Screen<TextInput autoFocus value={createName} onChange={(event) => onCreateName(event.target.value)} maxLength={100} placeholder="Single Line Diagram" required /></label>
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={() => onCreateOpen(false)}>ยกเลิก</Button><Button><FilePlus2 size={17} /> สร้าง Draft</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  </div>;
}

export function ScadaCanvas({ screenId, design, editable, devices, latestByDevice, catalogs = {}, versions, canPublish, onDesignChange, onRollback, hideInspector, showMinimap = true, locked = false }: {
  screenId?: string; design: ScadaDesign; editable: boolean; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; catalogs?: Record<string, Record<string, PointMeta>>; versions: ScadaScreenVersion[]; canPublish: boolean; onDesignChange: (design: ScadaDesign) => void; onRollback: (version: ScadaScreenVersion) => Promise<void>; hideInspector?: boolean; showMinimap?: boolean; locked?: boolean;
}) {
  const [nodes, setNodes] = useState<FlowNode[]>(() => design.nodes.map((node) => ({ ...node, zIndex: node.zIndex ?? (node.type === "section" || node.type === "group" ? -1 : 0), extent: node.parentId ? "parent" as const : undefined, style: node.width && node.height ? { width: node.width, height: node.height } : undefined })));
  const [edges, setEdges] = useState<FlowEdge[]>(() => design.edges.map((edge) => ({ ...edge, animated: false })));
  const [viewport, setViewport] = useState<Viewport>(design.viewport);
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [editingID, setEditingID] = useState("");
  const [historyTick, setHistoryTick] = useState(0);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null);
  const [imageError, setImageError] = useState("");
  const stageRef = useRef<HTMLDivElement>(null);
  const flowRef = useRef<ReactFlowInstance<FlowNode, FlowEdge> | null>(null);
  // ponytail: full ScadaDesign snapshots per undo step, capped at 50 -- fine at builder scale (a
  // few hundred nodes/edges), a diff-based history would only earn its keep past that.
  const historyRef = useRef<{ past: { nodes: FlowNode[]; edges: FlowEdge[] }[]; future: { nodes: FlowNode[]; edges: FlowEdge[] }[] }>({ past: [], future: [] });
  const clipboardRef = useRef<FlowNode[]>([]);
  const pasteCountRef = useRef(0);
  const selectedID = selectedIDs.length === 1 ? selectedIDs[0] : "";
  const selected = nodes.find((node) => node.id === selectedID);
  const selectedNodes = nodes.filter((node) => selectedIDs.includes(node.id));
  const selectionLocked = selectedNodes.length > 0 && selectedNodes.every((node) => node.data.locked);
  const selectionProtected = selectedNodes.some((node) => node.data.locked || inheritedFlag(node, "locked"));
  const canUngroup = selectedNodes.some((node) => node.type === "group");

  function emit(nextNodes: FlowNode[], nextEdges: FlowEdge[], nextViewport = viewport) {
    onDesignChange({
      version: 1,
      nodes: nextNodes.map((node) => {
        const { latest: _latest, latestByDevice: _latestByDevice, editing: _editing, onEditCommit: _onEditCommit, onEditCancel: _onEditCancel, ...data } = node.data;
        const width = node.measured?.width ?? node.width;
        const height = node.measured?.height ?? node.height;
        return { id: node.id, type: node.type as ScadaNodeType, position: node.position, data, ...(width ? { width } : {}), ...(height ? { height } : {}), ...(node.parentId ? { parentId: node.parentId } : {}), ...(node.zIndex ? { zIndex: node.zIndex } : {}) };
      }),
      edges: nextEdges.map((edge) => ({ id: edge.id, source: edge.source, target: edge.target, type: edge.type === "default" ? "default" : "smoothstep" })),
      viewport: nextViewport,
    });
  }

  // Undo/redo checkpoint. Called before discrete structural actions (add/remove/connect/
  // align/duplicate/paste/inline-edit/drag-start) -- not on every keystroke or drag pixel,
  // so a single undo reverts one meaningful action instead of one character or one frame.
  function pushHistory() {
    historyRef.current.past.push({ nodes, edges });
    if (historyRef.current.past.length > 50) historyRef.current.past.shift();
    historyRef.current.future = [];
    setHistoryTick((tick) => tick + 1);
  }

  function undo() {
    if (!editable) return;
    const previous = historyRef.current.past.pop();
    if (!previous) return;
    historyRef.current.future.push({ nodes, edges });
    setNodes(previous.nodes);
    setEdges(previous.edges);
    setHistoryTick((tick) => tick + 1);
    emit(previous.nodes, previous.edges);
  }

  function redo() {
    if (!editable) return;
    const next = historyRef.current.future.pop();
    if (!next) return;
    historyRef.current.past.push({ nodes, edges });
    setNodes(next.nodes);
    setEdges(next.edges);
    setHistoryTick((tick) => tick + 1);
    emit(next.nodes, next.edges);
  }

  function nodesChanged(changes: NodeChange<FlowNode>[]) {
    if (!editable) return;
    const allowed = changes.filter((change) => change.type !== "remove" || !nodes.some((node) => node.id === change.id && (node.data.locked || inheritedFlag(node, "locked"))));
    if (allowed.some((change) => change.type === "remove")) pushHistory();
    const next = applyNodeChanges(allowed, nodes);
    setNodes(next);
    emit(next, edges);
  }

  function edgesChanged(changes: EdgeChange<FlowEdge>[]) {
    if (!editable) return;
    if (changes.some((change) => change.type === "remove")) pushHistory();
    const next = applyEdgeChanges(changes, edges);
    setEdges(next);
    emit(nodes, next);
  }

  function connect(connection: Connection) {
    if (!editable || !connection.source || !connection.target || connection.source === connection.target) return;
    if (edges.some((edge) => edge.source === connection.source && edge.target === connection.target)) return;
    pushHistory();
    const next = addEdge({ ...connection, id: `edge-${crypto.randomUUID()}`, type: "smoothstep", animated: false }, edges);
    setEdges(next);
    emit(nodes, next);
  }

  function addNode(type: ScadaNodeType) {
    if (!editable) return;
    if (paletteEntries.find((entry) => entry.type === type)?.requiresDevice && devices.length === 0) return;
    pushHistory();
    const id = `${type}-${crypto.randomUUID()}`;
    const firstPointKey = devices[0] ? Object.keys(latestByDevice[devices[0].id]?.dataItemMap || {}).sort()[0] : undefined;
    const firstMeta = devices[0] && firstPointKey ? pointMeta(catalogs[devices[0].id] || {}, firstPointKey) : undefined;
    const binding = devices[0] ? { deviceId: devices[0].id, pointKey: firstPointKey || "", unit: firstMeta?.unit || "", decimals: firstMeta?.decimals ?? 2 } : undefined;
    const defaults: Record<ScadaNodeType, ScadaNodeData> = {
      equipment: { label: "Inverter", equipmentKind: "inverter" },
      metric: { label: "Active power", binding, displayType: "text", minValue: 0, maxValue: 100 },
      label: { label: "Section label", textColor: "#0f172a", fontSize: 18, fontWeight: "bold" },
      shape: { label: "Shape", shapeKind: "rectangle" },
      section: { label: "Equipment group" },
      group: { label: "Group", borderEnabled: true, borderColor: "#64748b", borderStyle: "dashed" },
      led: { label: "Running", binding, onValue: 1 },
      clock: { label: "Plant time", timezone: "Asia/Bangkok" },
      image: { label: "Reference image", imageUrl: "" },
      table: { label: "Measurements", items: [{ label: "Active power", binding: binding! }] },
      alarms: { label: "Threshold alarms", items: [{ label: "Active power", binding: binding!, minAlarm: 0, maxAlarm: 100 }] },
      ticker: { label: "Message", text: "Plant operating normally" },
      "device-summary": { label: "Device parameters", deviceId: devices[0]?.id },
    };
    const dimensions: { width?: number; height?: number } = type === "section" ? { width: 360, height: 220 } : type === "image" ? { width: 260, height: 160 } : type === "device-summary" ? { width: 300, height: 240 } : type === "table" || type === "alarms" ? { width: 280, height: 180 } : type === "shape" ? { width: 140, height: 90 } : {};
    const nextNode: FlowNode = { id, type, position: { x: 100 + nodes.length * 28, y: 100 + nodes.length * 24 }, data: defaults[type], zIndex: type === "section" || type === "group" ? -1 : 0, ...dimensions, ...(dimensions.width && dimensions.height ? { style: dimensions } : {}) };
    const next = [...nodes, nextNode];
    setNodes(next);
    setSelectedIDs([id]);
    emit(next, edges);
  }

  function updateSelected(data: ScadaNodeData) {
    const next = nodes.map((node) => node.id === selectedID ? { ...node, data } : node);
    setNodes(next);
    emit(next, edges);
  }

  function updateSelection(patch: Partial<ScadaNodeData>) {
    if (!selectedIDs.length) return;
    pushHistory();
    const next = nodes.map((node) => selectedIDs.includes(node.id) ? { ...node, data: { ...node.data, ...patch } } : node);
    setNodes(next);
    emit(next, edges);
  }

  function selectOnly(id: string) {
    setNodes((current) => current.map((node) => ({ ...node, selected: node.id === id })));
    setSelectedIDs([id]);
  }

  function inheritedFlag(node: FlowNode, flag: "locked" | "hidden") {
    let current = node;
    const visited = new Set<string>();
    while (current.parentId && !visited.has(current.parentId)) {
      visited.add(current.parentId);
      const parent = nodes.find((item) => item.id === current.parentId);
      if (!parent) break;
      if (parent.data[flag]) return true;
      current = parent;
    }
    return false;
  }

  function selectionWithDescendants(ids: string[]) {
    const included = new Set(ids);
    let changed = true;
    while (changed) {
      changed = false;
      for (const node of nodes) if (node.parentId && included.has(node.parentId) && !included.has(node.id)) { included.add(node.id); changed = true; }
    }
    return included;
  }

  function removeSelection() {
    if (!editable || selectedIDs.length === 0 || selectionProtected) return;
    pushHistory();
    const removed = selectionWithDescendants(selectedIDs);
    const nextNodes = nodes.filter((node) => !removed.has(node.id));
    const nextEdges = edges.filter((edge) => !removed.has(edge.source) && !removed.has(edge.target));
    setNodes(nextNodes); setEdges(nextEdges); setSelectedIDs([]); emit(nextNodes, nextEdges);
  }

  function alignSelection(edge: "left" | "center-h" | "right" | "top" | "middle-v" | "bottom") {
    if (!editable || selectedIDs.length < 2) return;
    const targets = nodes.filter((node) => selectedIDs.includes(node.id));
    const bounds = getNodesBounds(targets);
    pushHistory();
    const next = nodes.map((node) => {
      if (!selectedIDs.includes(node.id)) return node;
      const width = node.measured?.width ?? node.width ?? 0;
      const height = node.measured?.height ?? node.height ?? 0;
      const position = { ...node.position };
      if (edge === "left") position.x = bounds.x;
      else if (edge === "right") position.x = bounds.x + bounds.width - width;
      else if (edge === "center-h") position.x = bounds.x + bounds.width / 2 - width / 2;
      else if (edge === "top") position.y = bounds.y;
      else if (edge === "bottom") position.y = bounds.y + bounds.height - height;
      else position.y = bounds.y + bounds.height / 2 - height / 2;
      return { ...node, position };
    });
    setNodes(next);
    emit(next, edges);
  }

  function cloneNodes(source: FlowNode[], offset: number): FlowNode[] {
    const idMap = new Map(source.map((node) => [node.id, `${node.type}-${crypto.randomUUID()}`]));
    return source.map((node) => ({
      ...node,
      id: idMap.get(node.id)!,
      parentId: node.parentId ? idMap.get(node.parentId) || node.parentId : undefined,
      position: node.parentId && idMap.has(node.parentId) ? node.position : { x: node.position.x + offset, y: node.position.y + offset },
      selected: !node.parentId || !idMap.has(node.parentId),
    }));
  }

  function copyableSelection() {
    const included = selectionWithDescendants(selectedIDs);
    return nodes.filter((node) => included.has(node.id));
  }

  function duplicateSelection() {
    if (!editable || selectedIDs.length === 0) return;
    pushHistory();
    const copies = cloneNodes(copyableSelection(), 24);
    const next = [...nodes.map((node) => ({ ...node, selected: false })), ...copies];
    setNodes(next);
    setSelectedIDs(copies.filter((node) => node.selected).map((node) => node.id));
    emit(next, edges);
  }

  function copySelection() {
    if (selectedIDs.length === 0) return;
    clipboardRef.current = copyableSelection();
    pasteCountRef.current = 0;
  }

  function pasteClipboard() {
    if (!editable || clipboardRef.current.length === 0) return;
    pasteCountRef.current += 1;
    pushHistory();
    const copies = cloneNodes(clipboardRef.current, 24 * pasteCountRef.current);
    const next = [...nodes.map((node) => ({ ...node, selected: false })), ...copies];
    setNodes(next);
    setSelectedIDs(copies.filter((node) => node.selected).map((node) => node.id));
    emit(next, edges);
  }

  function pasteToReplace() {
    if (!editable || selectedIDs.length !== clipboardRef.current.length || !selectedIDs.length) return;
    pushHistory();
    const sources = clipboardRef.current;
    const next = nodes.map((node) => {
      const index = selectedIDs.indexOf(node.id);
      if (index < 0) return node;
      const source = sources[index];
      return { ...node, type: source.type, data: { ...source.data }, width: source.width, height: source.height, style: source.style };
    });
    setNodes(next);
    emit(next, edges);
  }

  function localBounds(targets: FlowNode[]) {
    const x = Math.min(...targets.map((node) => node.position.x));
    const y = Math.min(...targets.map((node) => node.position.y));
    const right = Math.max(...targets.map((node) => node.position.x + (node.measured?.width ?? node.width ?? 160)));
    const bottom = Math.max(...targets.map((node) => node.position.y + (node.measured?.height ?? node.height ?? 80)));
    return { x, y, width: right - x, height: bottom - y };
  }

  function wrapSelection(type: "group" | "section") {
    const targets = nodes.filter((node) => selectedIDs.includes(node.id));
    if (!editable || targets.length < 2 || targets.some((node) => node.parentId !== targets[0].parentId)) return;
    pushHistory();
    const padding = type === "section" ? 32 : 12;
    const bounds = localBounds(targets);
    const id = `${type}-${crypto.randomUUID()}`;
    const container: FlowNode = {
      id, type, parentId: targets[0].parentId, extent: targets[0].parentId ? "parent" : undefined,
      position: { x: bounds.x - padding, y: bounds.y - padding }, width: bounds.width + padding * 2, height: bounds.height + padding * 2,
      style: { width: bounds.width + padding * 2, height: bounds.height + padding * 2 }, zIndex: Math.min(...targets.map((node) => node.zIndex ?? 0)) - 1,
      data: type === "section"
        ? { label: "New section", backgroundColor: "#ffffff", borderColor: "#94a3b8", borderStyle: "solid", borderEnabled: true }
        : { label: "Group", borderColor: "#64748b", borderStyle: "dashed", borderEnabled: true },
      selected: true,
    };
    const next = nodes.map((node) => selectedIDs.includes(node.id) ? {
      ...node, parentId: id, extent: "parent" as const, position: { x: node.position.x - bounds.x + padding, y: node.position.y - bounds.y + padding }, selected: false,
    } : node);
    const parentIndex = container.parentId ? next.findIndex((node) => node.id === container.parentId) : -1;
    next.splice(parentIndex + 1, 0, container);
    setNodes(next); setSelectedIDs([id]); emit(next, edges);
  }

  function ungroupSelection() {
    const containers = nodes.filter((node) => selectedIDs.includes(node.id) && node.type === "group");
    if (!editable || containers.length === 0) return;
    pushHistory();
    const ids = new Set(containers.map((node) => node.id));
    const byID = new Map(containers.map((node) => [node.id, node]));
    const revealed: string[] = [];
    const next = nodes.filter((node) => !ids.has(node.id)).map((node) => {
      if (!node.parentId || !ids.has(node.parentId)) return node;
      const parent = byID.get(node.parentId)!;
      revealed.push(node.id);
      return { ...node, parentId: parent.parentId, extent: parent.parentId ? "parent" as const : undefined, position: { x: parent.position.x + node.position.x, y: parent.position.y + node.position.y }, selected: true };
    });
    setNodes(next); setSelectedIDs(revealed); emit(next, edges);
  }

  function changeLayerOrder(front: boolean) {
    if (!selectedIDs.length) return;
    pushHistory();
    const zValues = nodes.map((node) => node.zIndex ?? 0);
    const nextZ = (front ? Math.max(...zValues, 0) + 1 : Math.min(...zValues, 0) - 1);
    const next = nodes.map((node) => selectedIDs.includes(node.id) ? { ...node, zIndex: nextZ } : node);
    setNodes(next); emit(next, edges);
  }

  function unlockAll() {
    pushHistory();
    const next = nodes.map((node) => node.data.locked ? { ...node, data: { ...node.data, locked: false } } : node);
    setNodes(next); emit(next, edges);
  }

  function renameSelection() {
    if (!selected) return;
    const label = window.prompt("Rename object", selected.data.label)?.trim();
    if (label) updateSelected({ ...selected.data, label: label.slice(0, 100) });
  }

  async function uploadImage(file: File, targetID?: string, position?: { x: number; y: number }) {
    if (!screenId || !file.type.startsWith("image/")) return;
    if (file.size > 2 * 1024 * 1024) { setImageError("รูปต้องมีขนาดไม่เกิน 2 MB"); return; }
    setImageError("");
    const form = new FormData(); form.append("image", file);
    const response = await api(`/api/v1/scada/screens/${encodeURIComponent(screenId)}/images`, { method: "POST", headers: { "X-CSRF-Token": csrfToken() }, body: form });
    if (!response.ok) { setImageError("อัปโหลดรูปไม่สำเร็จ รองรับ PNG, JPEG และ WebP ไม่เกิน 2 MB"); return; }
    const { url } = await response.json() as { url: string };
    pushHistory();
    if (targetID) {
      const next = nodes.map((node) => node.id === targetID ? { ...node, data: { ...node.data, imageUrl: url, label: file.name.slice(0, 100) } } : node);
      setNodes(next); emit(next, edges); return;
    }
    const id = `image-${crypto.randomUUID()}`;
    const node: FlowNode = { id, type: "image", position: position || { x: 120, y: 120 }, width: 260, height: 160, style: { width: 260, height: 160 }, data: { label: file.name.slice(0, 100), imageUrl: url } };
    const next = [...nodes.map((item) => ({ ...item, selected: false })), { ...node, selected: true }];
    setNodes(next); setSelectedIDs([id]); emit(next, edges);
  }

  function commitEdit(id: string, patch: Partial<ScadaNodeData>) {
    pushHistory();
    const next = nodes.map((node) => node.id === id ? { ...node, data: { ...node.data, ...patch } } : node);
    setNodes(next);
    setEditingID("");
    emit(next, edges);
  }

  // Window-level so shortcuts fire regardless of which canvas element has focus. Rebinding on
  // every nodes/edges/selectedIDs change keeps the closures fresh (cheap: add/removeEventListener).
  useEffect(() => {
    if (!editable) return;
    function isTypingTarget(target: EventTarget | null) {
      return target instanceof HTMLElement && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable);
    }
    function onKeyDown(event: KeyboardEvent) {
      if (isTypingTarget(event.target) || !(event.ctrlKey || event.metaKey)) return;
      const key = event.key.toLowerCase();
      if (key === "c") copySelection();
      else if (key === "v") pasteClipboard();
      else if (key === "d") { event.preventDefault(); duplicateSelection(); }
      else if (key === "z" && event.shiftKey) { event.preventDefault(); redo(); }
      else if (key === "z") { event.preventDefault(); undo(); }
      else if (key === "y") { event.preventDefault(); redo(); }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [editable, nodes, edges, selectedIDs]);

  useEffect(() => {
    if (!contextMenu) return;
    const close = () => setContextMenu(null);
    window.addEventListener("click", close);
    window.addEventListener("blur", close);
    return () => { window.removeEventListener("click", close); window.removeEventListener("blur", close); };
  }, [contextMenu]);

  return <div className={editable ? "scada-workbench editing scada-dark" : hideInspector ? "scada-workbench solo scada-dark" : "scada-workbench viewing scada-dark"}>
    {editable && <aside className="scada-palette"><header><Grid2X2 size={17} /><div><strong>Node palette</strong><small>เพิ่มองค์ประกอบลง fixed canvas</small></div></header>{paletteEntries.map(({ type, title, description, icon: Icon, requiresDevice }) => <Button variant="bare" key={type} onClick={() => addNode(type)} disabled={requiresDevice && devices.length === 0} title={requiresDevice && devices.length === 0 ? "Plant นี้ยังไม่มี Device" : undefined}><Icon size={18} /><span><strong>{title}</strong><small>{description}</small></span></Button>)}<div className="palette-note"><Workflow size={16} /><p>ใช้ Edge ของ React Flow สำหรับเส้นและลูกศรระหว่าง Node</p></div></aside>}
    <section className="scada-stage-shell" ref={stageRef}>
      <div className="scada-stage-toolbar">
        <span>{editable ? "Draft canvas · Snap 20px" : "Operational viewer · Read only"}</span>
        <div className="flex items-center gap-1">
          {editable && <Button variant="icon" disabled={historyRef.current.past.length === 0} onClick={undo} title="เลิกทำ (Ctrl+Z)" aria-label="เลิกทำ"><Undo2 size={16} /></Button>}
          {editable && <Button variant="icon" disabled={historyRef.current.future.length === 0} onClick={redo} title="ทำซ้ำ (Ctrl+Shift+Z)" aria-label="ทำซ้ำ"><Redo2 size={16} /></Button>}
          <Button variant="icon" onClick={() => void stageRef.current?.requestFullscreen()} title="เต็มจอ" aria-label="แสดง SCADA เต็มจอ"><Fullscreen size={17} /></Button>
        </div>
      </div>
      <div className="scada-stage" onDragOver={(event) => { if (editable && Array.from(event.dataTransfer.items).some((item) => item.type.startsWith("image/"))) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; } }} onDrop={(event) => { const file = event.dataTransfer.files[0]; if (!editable || !file?.type.startsWith("image/")) return; event.preventDefault(); const position = flowRef.current?.screenToFlowPosition({ x: event.clientX, y: event.clientY }); void uploadImage(file, undefined, position); }}>
        <ReactFlow<FlowNode, FlowEdge> nodes={nodes.map((node) => ({ ...node, style: { ...node.style, ...nodeVisualVars(node.data), opacity: editable && (node.data.hidden || inheritedFlag(node, "hidden")) ? .35 : 1 }, draggable: editable && !locked && !node.data.locked && !inheritedFlag(node, "locked"), hidden: !editable && (node.data.hidden || inheritedFlag(node, "hidden")), data: { ...node.data, latest: latestByDevice[node.data.binding?.deviceId || ""], latestByDevice, catalogs, builderMode: editable, editing: node.id === editingID, onEditCommit: (patch: Partial<ScadaNodeData>) => commitEdit(node.id, patch), onEditCancel: () => setEditingID("") } }))} edges={edges} nodeTypes={nodeTypes} onInit={(instance) => { flowRef.current = instance; }} onNodesChange={nodesChanged} onEdgesChange={edgesChanged} onConnect={connect} onNodeDragStart={() => { if (editable) pushHistory(); }} onSelectionChange={({ nodes: selectedNodes }) => setSelectedIDs((current) => { const next = selectedNodes.map((node) => node.id); return sameSelection(current, next) ? current : next; })} onNodeClick={(event, node) => { const parent = node.parentId ? nodes.find((item) => item.id === node.parentId && item.type === "group") : undefined; if (editable && parent && !event.altKey) selectOnly(parent.id); }} onNodeContextMenu={(event, node) => { if (!editable) return; event.preventDefault(); const parent = node.parentId ? nodes.find((item) => item.id === node.parentId && item.type === "group") : undefined; const targetID = parent?.id || node.id; if (!selectedIDs.includes(targetID)) selectOnly(targetID); setContextMenu({ x: Math.max(8, Math.min(event.clientX, window.innerWidth - 226)), y: Math.max(8, Math.min(event.clientY, window.innerHeight - 410)) }); }} onPaneClick={() => setContextMenu(null)} onNodeDoubleClick={(_, node) => { if (editable && (node.type === "label" || node.type === "section" || node.type === "group" || node.type === "ticker")) setEditingID(node.id); }} onMoveEnd={(_, next) => { setViewport(next); if (editable) emit(nodes, edges, next); }} defaultViewport={viewport} nodesDraggable={editable} nodesConnectable={editable} elementsSelectable={editable} deleteKeyCode={editable ? ["Backspace", "Delete"] : null} panOnDrag={!locked} panOnScroll={!locked} zoomOnScroll={!locked} zoomOnPinch={!locked} zoomOnDoubleClick={!locked} snapToGrid snapGrid={[20, 20]} fitView minZoom={0.1} maxZoom={4} attributionPosition="bottom-left">
          <Background variant={BackgroundVariant.Lines} gap={20} size={1} color="var(--line)" />
          {showMinimap && <MiniMap pannable zoomable nodeColor={(node) => node.type === "metric" ? "var(--action)" : node.type === "equipment" ? "var(--warning)" : "var(--muted)"} />}
          {!locked && <Controls showInteractive={false} />}
          {editable && selectedIDs.length > 0 && (
            <Panel position="top-center">
              <div className="scada-selection-toolbar">
                {selectedIDs.length >= 2 && <><Button variant="icon" compact onClick={() => alignSelection("left")} title="ชิดซ้าย" aria-label="จัดชิดซ้าย"><AlignHorizontalJustifyStart size={16} /></Button>
                <Button variant="icon" compact onClick={() => alignSelection("center-h")} title="กึ่งกลางแนวนอน" aria-label="จัดกึ่งกลางแนวนอน"><AlignHorizontalJustifyCenter size={16} /></Button>
                <Button variant="icon" compact onClick={() => alignSelection("right")} title="ชิดขวา" aria-label="จัดชิดขวา"><AlignHorizontalJustifyEnd size={16} /></Button>
                <span className="mx-1 h-5 w-px bg-line" />
                <Button variant="icon" compact onClick={() => alignSelection("top")} title="ชิดบน" aria-label="จัดชิดบน"><AlignVerticalJustifyStart size={16} /></Button>
                <Button variant="icon" compact onClick={() => alignSelection("middle-v")} title="กึ่งกลางแนวตั้ง" aria-label="จัดกึ่งกลางแนวตั้ง"><AlignVerticalJustifyCenter size={16} /></Button>
                <Button variant="icon" compact onClick={() => alignSelection("bottom")} title="ชิดล่าง" aria-label="จัดชิดล่าง"><AlignVerticalJustifyEnd size={16} /></Button>
                <span className="mx-1 h-5 w-px bg-line" /></>}
                <label title="Background color">Fill <input type="color" value={selected?.data.backgroundColor || "#ffffff"} onChange={(event) => updateSelection({ backgroundColor: event.target.value })} /></label>
                <label title="Border color">Stroke <input type="color" value={selected?.data.borderColor || "#94a3b8"} onChange={(event) => updateSelection({ borderColor: event.target.value })} /></label>
                <select aria-label="ชนิดเส้นขอบ" value={selected?.data.borderStyle || "solid"} onChange={(event) => updateSelection({ borderStyle: event.target.value as ScadaNodeData["borderStyle"] })}><option value="solid">Solid</option><option value="dashed">Dashed</option><option value="dotted">Dotted</option></select>
                <Button variant="text" compact onClick={() => updateSelection({ borderEnabled: !(selected?.data.borderEnabled ?? true) })}>{selected?.data.borderEnabled === false ? "Border off" : "Border on"}</Button>
                <Button variant="text" compact onClick={renameSelection}>Rename</Button>
                <Button variant="text" compact onClick={() => updateSelection({ hidden: !selected?.data.hidden })}>{selected?.data.hidden ? "Unhide" : "Hide"}</Button>
                <Button variant="text" compact onClick={() => updateSelection({ locked: !selectionLocked })}>{selectionLocked ? "Unlock" : "Lock"}</Button>
                <span className="px-1.5 text-xs font-bold text-ink-soft">{selectedIDs.length} รายการ</span>
              </div>
            </Panel>
          )}
        </ReactFlow>
      </div>
      {imageError && <div className="scada-upload-error">{imageError}</div>}
    </section>
    {contextMenu && <div className="scada-context-menu" style={{ left: contextMenu.x, top: contextMenu.y }} onClick={(event) => event.stopPropagation()} role="menu">
      <button onClick={() => { copySelection(); setContextMenu(null); }}>Copy</button>
      <button disabled={!clipboardRef.current.length} onClick={() => { pasteClipboard(); setContextMenu(null); }}>Paste</button>
      <button disabled={!clipboardRef.current.length || selectedIDs.length !== clipboardRef.current.length} onClick={() => { pasteToReplace(); setContextMenu(null); }}>Paste to replace</button>
      <span />
      <button disabled={selectionProtected} onClick={() => { removeSelection(); setContextMenu(null); }}>Delete</button>
      <button onClick={() => { changeLayerOrder(true); setContextMenu(null); }}>Bring to front</button>
      <button onClick={() => { changeLayerOrder(false); setContextMenu(null); }}>Send to back</button>
      <button onClick={() => { updateSelection({ locked: !selectionLocked }); setContextMenu(null); }}>{selectionLocked ? "Unlock" : "Lock"}</button>
      <button onClick={() => { unlockAll(); setContextMenu(null); }}>Unlock all objects</button>
      {selectedIDs.length >= 2 && !canUngroup && <><span /><button onClick={() => { wrapSelection("group"); setContextMenu(null); }}>Group</button><button onClick={() => { wrapSelection("section"); setContextMenu(null); }}>Wrap in new section</button></>}
      {canUngroup && <><span /><button onClick={() => { ungroupSelection(); setContextMenu(null); }}>Ungroup</button></>}
    </div>}
    {editable && <ScadaInspector selected={selected} devices={devices} latestByDevice={latestByDevice} catalogs={catalogs} versions={versions} canPublish={canPublish} onUpdate={updateSelected} onImageUpload={(file) => selected && uploadImage(file, selected.id)} onRollback={onRollback} />}
    {!editable && !hideInspector && <aside className="scada-inspector viewer-info"><header><History size={17} /><div><strong>Published history</strong><small>เวอร์ชัน immutable</small></div></header><VersionHistory versions={versions} canPublish={canPublish} onRollback={onRollback} /></aside>}
  </div>;
}

function VersionHistory({ versions, canPublish, onRollback }: { versions: ScadaScreenVersion[]; canPublish: boolean; onRollback: (version: ScadaScreenVersion) => Promise<void> }) {
  return <div className="version-list">{versions.map((version) => <div key={version.version}><span><strong>Version {version.version}</strong><small><Clock3 size={12} /> {formatDate(version.publishedAt)} · Draft {version.sourceDraftVersion}</small></span>{version.current ? <StatusTag tone="active">Current</StatusTag> : canPublish ? <Button variant="icon" onClick={() => void onRollback(version)} title={`Rollback ไป Version ${version.version}`} aria-label={`Rollback ไป Version ${version.version}`}><RotateCcw size={15} /></Button> : null}</div>)}{versions.length === 0 && <p className="muted-text">ยังไม่มี Published version</p>}</div>;
}

function SaveState({ state }: { state: "saved" | "dirty" | "saving" | "conflict" | "error" }) {
  const labels = { saved: "Saved", dirty: "Unsaved", saving: "Saving", conflict: "Conflict", error: "Save failed" };
  return <span className={`save-state ${state}`}>{state === "saving" ? <LoaderCircle className="spin" size={14} /> : state === "saved" ? <Save size={14} /> : <Activity size={14} />}{labels[state]}</span>;
}

function ScadaInspector({ selected, devices, latestByDevice, catalogs, versions, canPublish, onUpdate, onImageUpload, onRollback }: {
  selected?: FlowNode; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; catalogs: Record<string, Record<string, PointMeta>>; versions: ScadaScreenVersion[]; canPublish: boolean;
  onUpdate: (data: ScadaNodeData) => void; onImageUpload: (file: File) => void | Promise<void>; onRollback: (version: ScadaScreenVersion) => Promise<void>;
}) {
  if (!selected) return <aside className="scada-inspector"><header><History size={17} /><div><strong>Published history</strong><small>เลือก Node เพื่อแก้ไขคุณสมบัติ</small></div></header><VersionHistory versions={versions} canPublish={canPublish} onRollback={onRollback} /></aside>;
  const data = selected.data;
  const update = (patch: Partial<ScadaNodeData>) => onUpdate({ ...data, ...patch });
  const bound = selected.type === "metric" || selected.type === "led";
  const listed = selected.type === "table" || selected.type === "alarms";
  const deviceOnly = selected.type === "device-summary";
  return <aside className="scada-inspector">
    <header><Pencil size={17} /><div><strong>Node properties</strong><Tooltip><TooltipTrigger asChild><small className="cursor-help underline decoration-dotted">{selected.type} · {selected.id.slice(0, 12)}</small></TooltipTrigger><TooltipContent>Node type: {selected.type}<br />Full ID: {selected.id}</TooltipContent></Tooltip></div></header>
    <label>Label<TextInput value={selected.type === "metric" && data.binding ? pointMeta(catalogs[data.binding.deviceId] || {}, data.binding.pointKey).displayName : data.label} maxLength={100} disabled={selected.type === "metric"} onChange={(event) => update({ label: event.target.value })} /></label>
    {selected.type === "equipment" && <label>Equipment type<Select value={data.equipmentKind || "inverter"} onValueChange={(value) => update({ equipmentKind: value as ScadaNodeData["equipmentKind"] })}><SelectTrigger aria-label="Equipment type"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="solar-panel">Solar panel</SelectItem><SelectItem value="inverter">Inverter</SelectItem><SelectItem value="meter">Meter</SelectItem><SelectItem value="grid">Grid</SelectItem></SelectContent></Select></label>}
    {bound && <BindingEditor binding={data.binding} devices={devices} latestByDevice={latestByDevice} catalogs={catalogs} onChange={(binding) => update({ binding })} />}
    {deviceOnly && <label>Device<Select value={data.deviceId || ""} onValueChange={(value) => update({ deviceId: value })}><SelectTrigger aria-label="Device"><SelectValue /></SelectTrigger><SelectContent>{devices.map((device) => <SelectItem key={device.id} value={device.id}>{device.name}</SelectItem>)}</SelectContent></Select></label>}
    {selected.type === "metric" && <><label>Display<Select value={data.displayType || "text"} onValueChange={(value) => update({ displayType: value as ScadaNodeData["displayType"] })}><SelectTrigger aria-label="Display"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="text">Text</SelectItem><SelectItem value="gauge">Gauge</SelectItem><SelectItem value="progress">Progress</SelectItem><SelectItem value="tank">Tank</SelectItem></SelectContent></Select></label><div className="inspector-grid"><NumberField label="Minimum" value={data.minValue} onChange={(minValue) => update({ minValue })} /><NumberField label="Maximum" value={data.maxValue} onChange={(maxValue) => update({ maxValue })} /></div></>}
    {selected.type === "led" && <NumberField label="On when value equals" value={data.onValue} onChange={(onValue) => update({ onValue })} />}
    {selected.type === "shape" && <label>Shape<Select value={data.shapeKind || "rectangle"} onValueChange={(value) => update({ shapeKind: value as ScadaNodeData["shapeKind"] })}><SelectTrigger aria-label="Shape"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="rectangle">Rectangle</SelectItem><SelectItem value="circle">Circle</SelectItem><SelectItem value="triangle">Triangle</SelectItem><SelectItem value="diamond">Diamond</SelectItem><SelectItem value="hexagon">Hexagon</SelectItem></SelectContent></Select></label>}
    {selected.type === "clock" && <label>Timezone<Select value={data.timezone || "Asia/Bangkok"} onValueChange={(value) => update({ timezone: value })}><SelectTrigger aria-label="Timezone"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="Asia/Bangkok">Asia/Bangkok</SelectItem><SelectItem value="UTC">UTC</SelectItem><SelectItem value="Asia/Singapore">Asia/Singapore</SelectItem><SelectItem value="Asia/Tokyo">Asia/Tokyo</SelectItem></SelectContent></Select></label>}
    {selected.type === "image" && <><label>Image URL<TextInput value={data.imageUrl || ""} maxLength={2048} placeholder="https://… or /images/…" onChange={(event) => update({ imageUrl: event.target.value })} /></label><label className="scada-image-picker">Upload from computer<input className="nodrag" type="file" accept="image/png,image/jpeg,image/webp" onChange={(event) => { const file = event.target.files?.[0]; if (file) void onImageUpload(file); event.currentTarget.value = ""; }} /><small>PNG, JPEG หรือ WebP ไม่เกิน 2 MB</small></label></>}
    {selected.type === "ticker" && <label>Message<TextInput value={data.text || ""} maxLength={200} onChange={(event) => update({ text: event.target.value })} /></label>}
    {selected.type === "label" && <><div className="inspector-grid"><label>Text color<input className="scada-color-input" type="color" value={data.textColor || "#0f172a"} onChange={(event) => update({ textColor: event.target.value })} /></label><NumberField label="Font size" value={data.fontSize ?? 18} min={8} max={144} onChange={(fontSize) => update({ fontSize })} /></div><label>Weight<Select value={data.fontWeight || "bold"} onValueChange={(value) => update({ fontWeight: value as ScadaNodeData["fontWeight"] })}><SelectTrigger aria-label="Text weight"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="normal">Normal</SelectItem><SelectItem value="bold">Bold</SelectItem></SelectContent></Select></label></>}
    {listed && <DataItemsEditor items={data.items || []} devices={devices} latestByDevice={latestByDevice} catalogs={catalogs} alarms={selected.type === "alarms"} onChange={(items) => update({ items })} />}
  </aside>;
}

function NumberField({ label, value, min, max, onChange }: { label: string; value?: number; min?: number; max?: number; onChange: (value?: number) => void }) {
  return <label>{label}<TextInput type="number" min={min} max={max} value={value == null ? "" : String(value)} onChange={(event) => onChange(event.target.value === "" ? undefined : Number(event.target.value))} /></label>;
}

function BindingEditor({ binding, devices, latestByDevice, catalogs, onChange }: { binding?: ScadaNodeData["binding"]; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; catalogs: Record<string, Record<string, PointMeta>>; onChange: (binding: NonNullable<ScadaNodeData["binding"]>) => void }) {
  const current = binding || { deviceId: devices[0]?.id || "", pointKey: "", unit: "kW", decimals: 1 };
  const options = bindingPointOptions(current.deviceId, latestByDevice, catalogs, current.pointKey);
  function changeDevice(deviceId: string) {
    const first = bindingPointOptions(deviceId, latestByDevice, catalogs)[0];
    onChange({ ...current, deviceId, pointKey: first?.value || "", unit: first?.unit || "", decimals: first?.decimals ?? 2 });
  }
  // Picking a parameter carries its engineering unit across, so the Unit field
  // stops being something to remember and retype for every node.
  function changePoint(pointKey: string) {
    const meta = options.find((option) => option.value === pointKey);
    onChange({ ...current, pointKey, unit: meta?.unit || "", decimals: meta?.decimals ?? 2 });
  }
  return <div className="binding-editor">
    <label>Device<Select value={current.deviceId} onValueChange={changeDevice}><SelectTrigger aria-label="Device"><SelectValue /></SelectTrigger><SelectContent>{devices.map((device) => <SelectItem key={device.id} value={device.id}>{device.name}</SelectItem>)}</SelectContent></Select></label>
    <label>Parameter<Select value={current.pointKey} onValueChange={changePoint}>
      <SelectTrigger aria-label="Parameter"><SelectValue placeholder={options.length === 0 ? "ยังไม่มีข้อมูล" : "-- เลือก parameter --"} /></SelectTrigger>
      <SelectContent>{options.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}{option.tag ? ` · ${option.tag}` : ""}</SelectItem>)}</SelectContent>
    </Select></label>
    <div className="inspector-grid"><label>Unit<TextInput value={pointMeta(catalogs[current.deviceId] || {}, current.pointKey).unit || current.unit} readOnly /></label><label>Decimals<TextInput type="number" value={String(pointMeta(catalogs[current.deviceId] || {}, current.pointKey).decimals ?? current.decimals)} readOnly /></label></div>
  </div>;
}

/**
 * Parameter choices for one device: register display name as the label, Modbus
 * address as the tag, and the telemetry key as the stored value.
 *
 * Driven by the keys the device is actually reporting (dataItemMap), not by the
 * whole catalog -- binding a node to a register the gateway never sends would
 * only ever render "No data". A key with no metadata row keeps showing its raw
 * address rather than disappearing, and `selected` keeps an already-saved
 * binding in the list even if the device has stopped reporting it, so opening
 * the inspector cannot silently rewrite a published screen.
 */
function bindingPointOptions(
  deviceId: string,
  latestByDevice: Record<string, LatestTelemetry>,
  catalogs: Record<string, Record<string, PointMeta>>,
  selected?: string,
) {
  const catalog = catalogs[deviceId] ?? {};
  const keys = Object.keys(latestByDevice[deviceId]?.dataItemMap || {});
  if (selected && !keys.includes(selected)) keys.unshift(selected);
  return keys
    .map((key) => {
      const meta = pointMeta(catalog, key);
      return { value: key, label: meta.displayName, tag: meta.tag === key ? "" : meta.tag, unit: meta.unit, decimals: meta.decimals };
    })
    .sort((a, b) => a.label.localeCompare(b.label));
}

function DataItemsEditor({ items, devices, latestByDevice, catalogs, alarms, onChange }: { items: ScadaDataItem[]; devices: Device[]; latestByDevice: Record<string, LatestTelemetry>; catalogs: Record<string, Record<string, PointMeta>>; alarms: boolean; onChange: (items: ScadaDataItem[]) => void }) {
  const fallbackDeviceId = devices[0]?.id || "";
  const fallbackPointKey = Object.keys(latestByDevice[fallbackDeviceId]?.dataItemMap || {}).sort()[0] || "";
  const fallbackMeta = pointMeta(catalogs[fallbackDeviceId] || {}, fallbackPointKey);
  const fallback = { deviceId: fallbackDeviceId, pointKey: fallbackPointKey, unit: fallbackMeta.unit, decimals: fallbackMeta.decimals };
  const patch = (index: number, next: Partial<ScadaDataItem>) => onChange(items.map((item, itemIndex) => itemIndex === index ? { ...item, ...next } : item));
  return <section className="scada-item-editor"><div className="item-editor-heading"><strong>{alarms ? "Alarm points" : "Table rows"}</strong><Button type="button" variant="icon" disabled={items.length >= 20 || devices.length === 0} onClick={() => onChange([...items, { label: `Point ${items.length + 1}`, binding: fallback }])} title="เพิ่ม point" aria-label="เพิ่ม point">+</Button></div>{items.map((item, index) => <div className="scada-item-row" key={`${index}-${item.binding.deviceId}`}><label>Label<TextInput value={item.label} maxLength={100} onChange={(event) => patch(index, { label: event.target.value })} /></label><BindingEditor binding={item.binding} devices={devices} latestByDevice={latestByDevice} catalogs={catalogs} onChange={(binding) => patch(index, { binding })} />{alarms && <div className="inspector-grid"><NumberField label="Low alarm" value={item.minAlarm} onChange={(minAlarm) => patch(index, { minAlarm })} /><NumberField label="High alarm" value={item.maxAlarm} onChange={(maxAlarm) => patch(index, { maxAlarm })} /></div>}<Button type="button" variant="text" danger disabled={items.length <= 1} onClick={() => onChange(items.filter((_, itemIndex) => itemIndex !== index))}>ลบแถว</Button></div>)}</section>;
}

// "group" lets NodeHandles reveal its dots on hover via Tailwind group-hover, without any
// JS hover-tracking state -- the class also carries the existing selected-state modifier.
function nodeClass(kind: string, selected?: boolean) {
  return `scada-node ${kind} group${selected ? " selected" : ""}`;
}

function nodeVisualVars(data: ScadaNodeData): CSSProperties {
  return {
    "--node-background": data.backgroundColor || "var(--scada-surface)",
    "--node-border-color": data.borderEnabled === false ? "transparent" : data.borderColor || "var(--scada-line)",
    "--node-border-style": data.borderStyle || "solid",
    "--node-text-color": data.textColor || "var(--scada-ink)",
    "--node-font-size": data.fontSize ? `${data.fontSize}px` : undefined,
    "--node-font-weight": data.fontWeight || undefined,
  } as CSSProperties;
}

// Shared double-click-to-edit input for Label/Section/Ticker nodes. "nodrag nopan" are React
// Flow's built-in escape hatches so typing/clicking inside doesn't start a node drag or pan.
function InlineTextEdit({ value, className, onCommit, onCancel }: { value: string; className?: string; onCommit: (value: string) => void; onCancel: () => void }) {
  const [draft, setDraft] = useState(value);
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => { inputRef.current?.focus(); inputRef.current?.select(); }, []);
  return <input
    ref={inputRef}
    className={`nodrag nopan ${className || ""}`}
    value={draft}
    maxLength={200}
    onChange={(event) => setDraft(event.target.value)}
    onKeyDown={(event) => {
      if (event.key === "Enter") { event.preventDefault(); onCommit(draft); }
      else if (event.key === "Escape") { event.preventDefault(); onCancel(); }
    }}
    onBlur={() => onCommit(draft)}
  />;
}

const inlineEditClass = "w-full rounded-[var(--radius-sm)] border border-focus bg-surface px-1.5 py-1 text-sm font-bold text-ink outline-none";

function EquipmentNode({ data, selected }: NodeProps<FlowNode>) {
  const icons = { "solar-panel": SunMedium, inverter: Zap, meter: CircleGauge, grid: Workflow };
  const Icon = icons[data.equipmentKind || "inverter"];
  return <div className={nodeClass("equipment", selected)}><NodeHandles selected={selected} /><Icon size={23} /><strong>{data.label}</strong><small>{data.equipmentKind}</small></div>;
}

function MetricNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = readBinding(data, data.binding);
  const meta = data.binding ? pointMeta(data.catalogs?.[data.binding.deviceId] || {}, data.binding.pointKey) : undefined;
  const min = data.minValue ?? 0; const max = data.maxValue ?? 100; const value = reading.value ?? min;
  return <div className={nodeClass("metric", selected)}><NodeHandles selected={selected} /><small>{meta?.displayName || data.label}</small>{(data.displayType === "gauge" || data.displayType === "progress") && <MetricBar percent={((value - min) / Math.max(1, max - min)) * 100} />}{data.displayType === "tank" && <div className="metric-tank" aria-hidden="true"><span style={{ height: `${Math.max(0, Math.min(100, ((value - min) / Math.max(1, max - min)) * 100))}%` }} /></div>}<strong>{reading.missing ? "—" : reading.formatted} <em>{meta?.unit ?? data.binding?.unit}</em></strong><Quality reading={reading} /></div>;
}

// Gauge/progress metric fill. PrimeReact ProgressBar sets the width inline, so
// pt only has to supply the track and bar skin (the app runs unstyled).
function MetricBar({ percent }: { percent: number }) {
  return <ProgressBar
    value={Math.max(0, Math.min(100, Math.round(percent)))}
    showValue={false}
    pt={{
      root: { className: "metric-bar" },
      value: { className: "metric-bar-fill" },
    }}
  />;
}

function LabelNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("label", selected)}>
    <NodeHandles selected={selected} />
    {data.editing
      ? <InlineTextEdit className={inlineEditClass} value={data.label} onCommit={(value) => data.onEditCommit?.({ label: value || data.label })} onCancel={() => data.onEditCancel?.()} />
      : <strong>{data.label}</strong>}
  </div>;
}

function ShapeNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("shape", selected)}><NodeResizer minWidth={60} minHeight={40} isVisible={selected} /><NodeHandles selected={selected} /><div className={`shape-body ${data.shapeKind || "rectangle"}`}><strong>{data.label}</strong></div></div>;
}

function SectionNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("section", selected)}>
    <NodeResizer minWidth={180} minHeight={120} isVisible={selected} />
    <NodeHandles selected={selected} />
    {data.editing
      ? <InlineTextEdit className={`${inlineEditClass} max-w-[80%]`} value={data.label} onCommit={(value) => data.onEditCommit?.({ label: value || data.label })} onCancel={() => data.onEditCancel?.()} />
      : <strong>{data.label}</strong>}
  </div>;
}

function GroupNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("object-group", selected)}>
    <NodeResizer minWidth={80} minHeight={60} isVisible={selected} />
    {data.editing
      ? <InlineTextEdit className={`${inlineEditClass} max-w-[70%]`} value={data.label} onCommit={(value) => data.onEditCommit?.({ label: value || data.label })} onCancel={() => data.onEditCancel?.()} />
      : selected ? <small>{data.label}</small> : null}
  </div>;
}

function LedNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = readBinding(data, data.binding); const active = !reading.missing && reading.value === (data.onValue ?? 1);
  return <div className={nodeClass("led", selected)}><NodeHandles selected={selected} /><span className={active ? "led-lamp active" : "led-lamp"} aria-hidden="true" /><strong>{data.label}</strong><span className={`node-quality ${reading.missing ? "missing" : active ? "good" : "normal"}`}>{reading.missing ? "No data" : active ? "ON" : "OFF"}</span></div>;
}

function ClockNode({ data, selected }: NodeProps<FlowNode>) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => { const timer = window.setInterval(() => setNow(new Date()), 1000); return () => window.clearInterval(timer); }, []);
  let time = "Invalid timezone"; try { time = new Intl.DateTimeFormat("en-GB", { timeZone: data.timezone || "Asia/Bangkok", dateStyle: "medium", timeStyle: "medium" }).format(now); } catch { /* backend validation prevents this after save */ }
  return <div className={nodeClass("clock", selected)}><NodeHandles selected={selected} /><Clock3 size={18} /><strong>{data.label}</strong><time>{time}</time><small>{data.timezone || "Asia/Bangkok"}</small></div>;
}

function ImageNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("image", selected)}><NodeResizer minWidth={100} minHeight={80} isVisible={selected} /><NodeHandles selected={selected} />{data.imageUrl ? <img src={data.imageUrl} alt={data.label} /> : <div className="image-placeholder"><ImageIcon size={24} /><span>No image set</span></div>}<small>{data.label}</small></div>;
}

function DataTableNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("data-table", selected)}><NodeResizer minWidth={220} minHeight={130} isVisible={selected} /><NodeHandles selected={selected} /><strong>{data.label}</strong><table><tbody>{(data.items || []).map((item, index) => { const reading = readBinding(data, item.binding); return <tr key={`${index}-${item.binding.pointKey}`}><th>{item.label}</th><td>{reading.missing ? "—" : reading.formatted} <small>{item.binding.unit}</small></td></tr>; })}</tbody></table></div>;
}

function DeviceSummaryNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = data.latestByDevice?.[data.deviceId || ""];
  const entries = reading ? Object.entries(reading.dataItemMap).sort(([a], [b]) => a.localeCompare(b)) : [];
  return <div className={nodeClass("data-table", selected)}><NodeResizer minWidth={220} minHeight={130} isVisible={selected} /><NodeHandles selected={selected} /><strong>{data.label}</strong><table><tbody>{entries.map(([key, value]) => <tr key={key}><th>{key}</th><td>{Number.isFinite(value) ? value.toLocaleString() : "—"}</td></tr>)}{entries.length === 0 && <tr><td colSpan={2}>No data</td></tr>}</tbody></table></div>;
}

function AlarmListNode({ data, selected }: NodeProps<FlowNode>) {
  return <div className={nodeClass("alarm-list", selected)}><NodeResizer minWidth={240} minHeight={130} isVisible={selected} /><NodeHandles selected={selected} /><strong><BellRing size={15} /> {data.label}</strong><div className="alarm-items">{(data.items || []).map((item, index) => { const reading = readBinding(data, item.binding); const alarm = !reading.missing && ((item.minAlarm != null && reading.value! < item.minAlarm) || (item.maxAlarm != null && reading.value! > item.maxAlarm)); return <div key={`${index}-${item.binding.pointKey}`}><span>{item.label}<small>{reading.missing ? "—" : `${reading.formatted} ${item.binding.unit}`}</small></span><b className={reading.missing ? "missing" : alarm ? "alarm" : "normal"}>{reading.missing ? "No data" : alarm ? "Alarm" : "Normal"}</b></div>; })}</div></div>;
}

function TextTickerNode({ data, selected }: NodeProps<FlowNode>) {
  const reading = data.binding ? readBinding(data, data.binding) : undefined;
  return <div className={nodeClass("ticker", selected)}>
    <NodeHandles selected={selected} />
    <TextQuote size={15} />
    <strong>{data.label}</strong>
    {data.editing
      ? <InlineTextEdit className="w-full rounded-[var(--radius-sm)] border border-focus bg-surface px-1.5 py-1 text-xs text-ink outline-none" value={data.text || ""} onCommit={(value) => data.onEditCommit?.({ text: value })} onCancel={() => data.onEditCancel?.()} />
      : <span>{reading ? (reading.missing ? "No data" : `${reading.formatted} ${data.binding?.unit}`) : data.text || "—"}</span>}
  </div>;
}

// Handles stay mounted at all times (React Flow needs them to route existing edges) but are
// only visually + interactively active while the node is selected or hovered, matching Figma.
function NodeHandles({ selected }: { selected?: boolean }) {
  const visibility = selected
    ? "opacity-100 pointer-events-auto"
    : "opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto";
  const cls = `transition-opacity duration-150 ${visibility}`;
  return <>
    <Handle type="target" position={Position.Left} className={cls} />
    <Handle type="source" position={Position.Right} className={cls} />
    <Handle id="top" type="target" position={Position.Top} className={cls} />
    <Handle id="bottom" type="source" position={Position.Bottom} className={cls} />
  </>;
}

type BindingReading = { value?: number; formatted: string; missing: boolean; observedAt?: string };
function readBinding(data: RuntimeScadaNodeData, binding?: ScadaNodeData["binding"]): BindingReading {
  const latest = binding ? data.latestByDevice?.[binding.deviceId] || data.latest : undefined;
  const value = binding ? latest?.dataItemMap[binding.pointKey] : undefined;
  const missing = value == null || !Number.isFinite(value);
  const meta = binding ? pointMeta(data.catalogs?.[binding.deviceId] || {}, binding.pointKey) : undefined;
  const display = binding ? latest?.displayItemMap?.[binding.pointKey] : undefined;
  return { value, missing, formatted: missing ? "—" : display || value.toLocaleString(undefined, { minimumFractionDigits: meta?.decimals ?? binding?.decimals ?? 1, maximumFractionDigits: meta?.decimals ?? binding?.decimals ?? 1 }), observedAt: latest?.observedAt };
}

function Quality({ reading }: { reading: BindingReading }) {
  return <span className={reading.missing ? "node-quality missing" : "node-quality good"}>{reading.missing ? "No data" : `Observed ${reading.observedAt ? formatDate(reading.observedAt) : ""}`}</span>;
}

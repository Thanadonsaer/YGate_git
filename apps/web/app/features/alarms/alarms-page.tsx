"use client";

import { Check, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import { useRealtimeSocket } from "../../lib/realtime";
import type { AlarmEvent, AlarmNotifyRole, AlarmRule, AlarmRuleCondition, Device, Plant } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";

const severityStatusClass: Record<AlarmRule["severity"], string> = {
  warning: "status no_devices",
  major: "status degraded",
  critical: "status offline",
};

export function AlarmsPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [plantId, setPlantId] = useState("");
  const [tab, setTab] = useState<"log" | "rules">("log");
  const [rules, setRules] = useState<AlarmRule[]>([]);
  const [events, setEvents] = useState<AlarmEvent[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [notifyRoles, setNotifyRoles] = useState<AlarmNotifyRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<AlarmRule | "create" | null>(null);

  const loadPlants = useCallback(async () => {
    const response = await api("/api/v1/plants");
    if (response.ok) {
      const list = (await response.json()) as Plant[];
      setPlants(list);
      setPlantId((current) => current || list[0]?.id || "");
    }
  }, []);

  useEffect(() => { void loadPlants(); }, [loadPlants]);

  const loadAlarms = useCallback(async () => {
    if (!plantId) return;
    setLoading(true);
    setError("");
    try {
      const [rulesResponse, eventsResponse, devicesResponse, notifyRolesResponse] = await Promise.all([
        api(`/api/v1/plants/${plantId}/alarms/rules`),
        api(`/api/v1/plants/${plantId}/alarms/events`),
        api(`/api/v1/plants/${plantId}/devices`),
        api(`/api/v1/plants/${plantId}/alarms/notify-roles`),
      ]);
      if (rulesResponse.status === 403 || eventsResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู Alarm ของโรงไฟฟ้านี้");
      if (!rulesResponse.ok || !eventsResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูล Alarm ได้");
      setRules((await rulesResponse.json()) as AlarmRule[]);
      setEvents((await eventsResponse.json()) as AlarmEvent[]);
      setDevices(devicesResponse.ok ? (await devicesResponse.json()) as Device[] : []);
      setNotifyRoles(notifyRolesResponse.ok ? (await notifyRolesResponse.json()) as AlarmNotifyRole[] : []);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [plantId]);

  useEffect(() => { void loadAlarms(); }, [loadAlarms]);

  useRealtimeSocket(plantId, (message) => {
    if (message.type !== "alarm.event") return;
    setEvents((current) => {
      const existingIds = new Set(current.map((event) => event.id));
      const fresh = message.data.filter((event) => !existingIds.has(event.id));
      if (fresh.length === 0) return current;
      return [...fresh, ...current].sort((a, b) => b.id - a.id);
    });
  }, Boolean(plantId));

  async function deleteRule(rule: AlarmRule) {
    if (!window.confirm(`ลบกฎแจ้งเตือน “${rule.label}”?`)) return;
    const response = await api(`/api/v1/plants/${plantId}/alarms/rules/${encodeURIComponent(rule.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    if (response.ok) { toast.success(`ลบกฎ "${rule.label}" แล้ว`); await loadAlarms(); }
    else setError("ไม่สามารถลบกฎแจ้งเตือนได้");
  }

  async function acknowledge(event: AlarmEvent) {
    const response = await api(`/api/v1/plants/${plantId}/alarms/events/${event.id}/ack`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ note: "" }),
    });
    if (response.ok) { toast.success("Acknowledge alarm แล้ว"); await loadAlarms(); }
    else setError("ไม่สามารถ Acknowledge Alarm ได้");
  }

  function deviceName(deviceId: string) {
    return devices.find((device) => device.id === deviceId)?.name ?? deviceId;
  }

  function notifyRoleName(roleId?: string | null) {
    return roleId ? notifyRoles.find((role) => role.id === roleId)?.name ?? "-" : null;
  }

  function conditionThreshold(condition: { minValue?: number | null; maxValue?: number | null }) {
    return [condition.minValue != null ? `min ${condition.minValue}` : null, condition.maxValue != null ? `max ${condition.maxValue}` : null].filter(Boolean).join(" / ") || "-";
  }

  return (
    <div className="content plants-content">
      <div className="section-heading">
        <div><p>Operational alarms</p><h2>Alarm Monitoring</h2></div>
        <div className="heading-actions">
          <Select value={plantId} onValueChange={setPlantId}>
            <SelectTrigger className="w-48" aria-label="เลือกโรงไฟฟ้า"><SelectValue /></SelectTrigger>
            <SelectContent>{plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.name}</SelectItem>)}</SelectContent>
          </Select>
          <Tabs value={tab} onValueChange={(value) => setTab(value as "log" | "rules")}>
            <TabsList aria-label="มุมมอง Alarm">
              <TabsTrigger value="log">Log</TabsTrigger>
              <TabsTrigger value="rules">Rules</TabsTrigger>
            </TabsList>
          </Tabs>
          <Button variant="icon" onClick={() => void loadAlarms()} title="รีเฟรช" aria-label="รีเฟรช Alarm"><RefreshCw size={18} /></Button>
          {tab === "rules" && <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่มกฎ</Button>}
        </div>
      </div>
      {error && <p className="form-message error">{error}</p>}
      {tab === "log" ? (
        <div className="audit-table" role="table" aria-label="Alarm Log">
          <div className="audit-row audit-head" role="row"><span>เวลา</span><span>Point</span><span>ค่า / Threshold</span><span>Severity</span><span>สถานะ</span></div>
          {!loading && events.map((event) => (
            <div className="audit-row" role="row" key={event.id}>
              <div><strong>{formatDate(event.breachedAt)}</strong><small>{event.clearedAt ? `Cleared ${formatDate(event.clearedAt)}` : "เปิดอยู่"}</small></div>
              <div>
                <strong>{deviceName(event.deviceId)}</strong>
                <small>{(event.conditionSnapshot ?? []).map((c) => c.pointKey).join(", ") || "-"}</small>
              </div>
              <div>
                {(event.conditionSnapshot ?? []).map((c) => (
                  <div key={c.pointKey}><strong>{c.breached ? "⚠ " : ""}{c.value.toLocaleString()}</strong><small> {conditionThreshold(c)}</small></div>
                ))}
              </div>
              <span className={severityStatusClass[event.severity]}>{event.severity}</span>
              {event.acknowledgedBy ? (
                <span className="status active">Acked {event.acknowledgedAt ? formatDate(event.acknowledgedAt) : ""}</span>
              ) : (
                <Button variant="icon" onClick={() => void acknowledge(event)} title="Acknowledge" aria-label={`Acknowledge alarm ${event.id}`}><Check size={17} /></Button>
              )}
            </div>
          ))}
          {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
          {!loading && !error && events.length === 0 && <div className="table-state">ยังไม่มี Alarm Event สำหรับโรงไฟฟ้านี้</div>}
        </div>
      ) : (
        <div className="plant-table" role="table" aria-label="Alarm Rules">
          <div className="plant-row plant-head" role="row"><span>กฎ</span><span>Device / Point</span><span>Threshold</span><span>สถานะ</span><span aria-label="คำสั่ง" /></div>
          {!loading && rules.map((rule) => (
            <div className="plant-row" role="row" key={rule.id}>
              <div><strong>{rule.label}</strong><small>{rule.severity}{notifyRoleName(rule.notifyRoleId) ? ` · แจ้ง ${notifyRoleName(rule.notifyRoleId)}` : ""}</small></div>
              <div><span>{deviceName(rule.deviceId)}</span><small>{(rule.conditions ?? []).map((c) => c.pointKey).join(rule.conditionLogic === "OR" ? " หรือ " : " และ ")}</small></div>
              <div>{(rule.conditions ?? []).map((c) => <div key={c.pointKey}><span>{c.pointKey}</span> <small>{conditionThreshold(c)}</small></div>)}</div>
              <span className={rule.isActive ? "status active" : "status revoked"}>{rule.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</span>
              <div className="row-actions">
                <Button variant="icon" onClick={() => setEditor(rule)} title="แก้ไขกฎ" aria-label={`แก้ไข ${rule.label}`}><Pencil size={17} /></Button>
                <Button variant="icon" danger onClick={() => void deleteRule(rule)} title="ลบกฎ" aria-label={`ลบ ${rule.label}`}><Trash2 size={17} /></Button>
              </div>
            </div>
          ))}
          {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
          {!loading && !error && rules.length === 0 && <div className="table-state">ยังไม่มีกฎแจ้งเตือนสำหรับโรงไฟฟ้านี้</div>}
        </div>
      )}
      {editor && (
        <AlarmRuleEditor
          plantId={plantId}
          rule={editor === "create" ? undefined : editor}
          devices={devices}
          notifyRoles={notifyRoles}
          onClose={() => setEditor(null)}
          onSaved={() => { setEditor(null); void loadAlarms(); }}
        />
      )}
    </div>
  );
}

const NO_NOTIFY = "__none__";

type ConditionDraft = { pointKey: string; minValue: string; maxValue: string };

function draftFromCondition(condition: AlarmRuleCondition): ConditionDraft {
  return { pointKey: condition.pointKey, minValue: condition.minValue?.toString() ?? "", maxValue: condition.maxValue?.toString() ?? "" };
}

function AlarmRuleEditor({ plantId, rule, devices, notifyRoles, onClose, onSaved }: { plantId: string; rule?: AlarmRule; devices: Device[]; notifyRoles: AlarmNotifyRole[]; onClose: () => void; onSaved: () => void }) {
  const [deviceId, setDeviceId] = useState(rule?.deviceId ?? devices[0]?.id ?? "");
  const [label, setLabel] = useState(rule?.label ?? "");
  const [conditions, setConditions] = useState<ConditionDraft[]>(
    rule && rule.conditions?.length > 0 ? rule.conditions.map(draftFromCondition) : [{ pointKey: "", minValue: "", maxValue: "" }],
  );
  const [conditionLogic, setConditionLogic] = useState<AlarmRule["conditionLogic"]>(rule?.conditionLogic ?? "AND");
  const [severity, setSeverity] = useState<AlarmRule["severity"]>(rule?.severity ?? "warning");
  const [isActive, setIsActive] = useState(rule?.isActive ?? true);
  const [notifyRoleId, setNotifyRoleId] = useState(rule?.notifyRoleId ?? "");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  function optionalNumber(value: string) {
    return value.trim() === "" ? null : Number(value);
  }

  function updateCondition(index: number, patch: Partial<ConditionDraft>) {
    setConditions((current) => current.map((condition, i) => (i === index ? { ...condition, ...patch } : condition)));
  }

  function addCondition() {
    setConditions((current) => [...current, { pointKey: "", minValue: "", maxValue: "" }]);
  }

  function removeCondition(index: number) {
    setConditions((current) => current.filter((_, i) => i !== index));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const notifyRoleValue = notifyRoleId === "" ? null : notifyRoleId;
      const conditionsPayload = conditions.map((condition) => ({
        pointKey: condition.pointKey, minValue: optionalNumber(condition.minValue), maxValue: optionalNumber(condition.maxValue),
      }));
      const body = rule
        ? { label, conditionLogic, conditions: conditionsPayload, severity, isActive, notifyRoleId: notifyRoleValue }
        : { deviceId, label, conditionLogic, conditions: conditionsPayload, severity, notifyRoleId: notifyRoleValue };
      const response = await api(rule ? `/api/v1/plants/${plantId}/alarms/rules/${encodeURIComponent(rule.id)}` : `/api/v1/plants/${plantId}/alarms/rules`, {
        method: rule ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(body),
      });
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์บันทึกกฎแจ้งเตือนนี้");
      if (!response.ok) throw new Error("ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้");
      onSaved();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>Alarm rule</DialogDescription><DialogTitle>{rule ? "แก้ไขกฎแจ้งเตือน" : "เพิ่มกฎแจ้งเตือน"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            {!rule && (
              <label className="full-field">Device
                <Select value={deviceId} onValueChange={setDeviceId}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>{devices.map((device) => <SelectItem key={device.id} value={device.id}>{device.name}</SelectItem>)}</SelectContent>
                </Select>
              </label>
            )}
            <label className="full-field">ชื่อกฎ<input autoFocus value={label} onChange={(event) => setLabel(event.target.value)} maxLength={200} required /></label>

            <div className="full-field alarm-conditions">
              <div className="alarm-conditions-head">
                <span>Criteria (point + threshold) — ต้องเข้าเงื่อนไขตาม logic ด้านล่าง</span>
                {conditions.length > 1 && (
                  <Select value={conditionLogic} onValueChange={(value) => setConditionLogic(value as AlarmRule["conditionLogic"])}>
                    <SelectTrigger className="w-28" aria-label="Logic ระหว่างเงื่อนไข"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="AND">ต้องครบทุกข้อ (AND)</SelectItem>
                      <SelectItem value="OR">เข้าข้อใดข้อหนึ่ง (OR)</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              </div>
              {conditions.map((condition, index) => (
                <div className="alarm-condition-row" key={index}>
                  <input placeholder="Point key" value={condition.pointKey} onChange={(event) => updateCondition(index, { pointKey: event.target.value })} maxLength={200} required />
                  <input type="number" step="any" placeholder="Min" value={condition.minValue} onChange={(event) => updateCondition(index, { minValue: event.target.value })} />
                  <input type="number" step="any" placeholder="Max" value={condition.maxValue} onChange={(event) => updateCondition(index, { maxValue: event.target.value })} />
                  <Button type="button" variant="icon" danger disabled={conditions.length <= 1} onClick={() => removeCondition(index)} title="ลบเงื่อนไขนี้" aria-label={`ลบเงื่อนไข ${index + 1}`}><Trash2 size={16} /></Button>
                </div>
              ))}
              <Button type="button" variant="secondary" compact onClick={addCondition}><Plus size={15} /> เพิ่มเงื่อนไข</Button>
            </div>

            <label className="full-field">Severity
              <Select value={severity} onValueChange={(value) => setSeverity(value as AlarmRule["severity"])}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="warning">Warning</SelectItem>
                  <SelectItem value="major">Major</SelectItem>
                  <SelectItem value="critical">Critical</SelectItem>
                </SelectContent>
              </Select>
            </label>
            <label className="full-field">แจ้งเตือนไปที่ Role
              <Select value={notifyRoleId || NO_NOTIFY} onValueChange={(value) => setNotifyRoleId(value === NO_NOTIFY ? "" : value)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_NOTIFY}>ไม่ต้องแจ้งเตือน</SelectItem>
                  {notifyRoles.map((role) => <SelectItem key={role.id} value={role.id}>{role.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </label>
            {rule && <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /><span>เปิดใช้งานกฎนี้</span></label>}
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

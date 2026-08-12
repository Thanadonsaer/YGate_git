"use client";

import { Check, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { Checkbox, FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import { loadRegisterCatalog, loadRegisterCatalogs, pointMeta, pointOptions, type PointMeta } from "../../lib/telemetry-history";
import { useRealtimeSocket } from "../../lib/realtime";
import type { AlarmEvent, AlarmNotifyRole, AlarmRule, AlarmRuleCondition, ConditionLogic, Device, EventLogbookEntry, Plant } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { DataTable, TableColumn } from "../../components/ui/data-table";

const severityTone: Record<AlarmRule["severity"], string> = {
  warning: "no_devices",
  major: "degraded",
  critical: "offline",
};

export function AlarmsPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [plantId, setPlantId] = useState("");
  const [tab, setTab] = useState<"log" | "logbook" | "rules">("log");
  const [rules, setRules] = useState<AlarmRule[]>([]);
  const [events, setEvents] = useState<AlarmEvent[]>([]);
  const [logbook, setLogbook] = useState<EventLogbookEntry[]>([]);
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
      const [rulesResponse, eventsResponse, devicesResponse, notifyRolesResponse, logbookResponse] = await Promise.all([
        api(`/api/v1/plants/${plantId}/alarms/rules`),
        api(`/api/v1/plants/${plantId}/alarms/events`),
        api(`/api/v1/plants/${plantId}/devices`),
        api(`/api/v1/plants/${plantId}/alarms/notify-roles`),
        api(`/api/v1/plants/${plantId}/alarms/logbook`),
      ]);
      if (rulesResponse.status === 403 || eventsResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู Alarm ของโรงไฟฟ้านี้");
      if (!rulesResponse.ok || !eventsResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูล Alarm ได้");
      setRules((await rulesResponse.json()) as AlarmRule[]);
      setEvents((await eventsResponse.json()) as AlarmEvent[]);
      setLogbook(logbookResponse.ok ? (await logbookResponse.json()) as EventLogbookEntry[] : []);
      setDevices(devicesResponse.ok ? (await devicesResponse.json()) as Device[] : []);
      setNotifyRoles(notifyRolesResponse.ok ? (await notifyRolesResponse.json()) as AlarmNotifyRole[] : []);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [plantId]);

  useEffect(() => { void loadAlarms(); }, [loadAlarms]);

  const [catalogs, setCatalogs] = useState<Record<string, Record<string, PointMeta>>>({});
  useEffect(() => {
    if (!plantId || devices.length === 0) return;
    const controller = new AbortController();
    // Best-effort: without it the tables fall back to raw address keys, which
    // is what they always showed.
    void loadRegisterCatalogs(plantId, devices, controller.signal).then(setCatalogs).catch(() => undefined);
    return () => controller.abort();
  }, [plantId, devices]);

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

  async function createLogbookEntry(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const response = await api(`/api/v1/plants/${plantId}/alarms/logbook`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({
        deviceId: String(form.get("deviceId") || ""), eventType: String(form.get("eventType") || "NOTE"),
        category: String(form.get("category") || ""), title: String(form.get("title") || ""),
        startsAt: new Date(String(form.get("startsAt"))).toISOString(), note: String(form.get("note") || ""), source: "MANUAL",
      }),
    });
    if (!response.ok) { setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เพิ่ม Event Logbook" : "ไม่สามารถบันทึก Event Logbook ได้"); return; }
    toast.success("บันทึก Event Logbook แล้ว");
    event.currentTarget.reset();
    await loadAlarms();
  }

  function deviceName(deviceId: string) {
    return devices.find((device) => device.id === deviceId)?.name ?? deviceId;
  }

  // Same display names the rule editor's Point dropdown offers, so a rule reads
  // the same in the table and the log as it did when it was written.
  function pointLabel(deviceId: string, key: string) {
    return pointMeta(catalogs[deviceId] ?? {}, key).displayName;
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
          <Tabs value={tab} onValueChange={(value) => setTab(value as "log" | "logbook" | "rules")}>
            <TabsList aria-label="มุมมอง Alarm">
              <TabsTrigger value="log">Log</TabsTrigger>
              <TabsTrigger value="logbook">Event Logbook</TabsTrigger>
              <TabsTrigger value="rules">Rules</TabsTrigger>
            </TabsList>
          </Tabs>
          <Button variant="icon" onClick={() => void loadAlarms()} title="รีเฟรช" aria-label="รีเฟรช Alarm"><RefreshCw size={18} /></Button>
          {tab === "rules" && <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่มกฎ</Button>}
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {tab === "log" ? (
        loading ? (
          <div className="table-state">กำลังโหลดข้อมูล</div>
        ) : (
          <DataTable value={events} dataKey="id" aria-label="Alarm Log" emptyMessage={<div className="table-state">{error ? "" : "ยังไม่มี Alarm Event สำหรับโรงไฟฟ้านี้"}</div>}>
            <TableColumn field="breachedAt" header="เวลา" sortable body={(event: AlarmEvent) => (
              <div className="grid gap-1"><strong>{formatDate(event.breachedAt)}</strong><small className="block text-[11px] text-ink-soft">{event.clearedAt ? `Cleared ${formatDate(event.clearedAt)}` : "เปิดอยู่"}</small></div>
            )} />
            <TableColumn header="Point" body={(event: AlarmEvent) => (
              <div className="grid gap-1"><strong>{deviceName(event.deviceId)}</strong><small className="block text-[11px] text-ink-soft">{(event.conditionSnapshot ?? []).map((c) => pointLabel(event.deviceId, c.pointKey)).join(", ") || "-"}</small></div>
            )} />
            <TableColumn header="ค่า / Threshold" body={(event: AlarmEvent) => (
              <div className="grid gap-1">
                {(event.conditionSnapshot ?? []).map((c) => (
                  <div key={c.pointKey}><strong>{c.breached ? "⚠ " : ""}{c.value.toLocaleString()}</strong><small> {conditionThreshold(c)}</small></div>
                ))}
              </div>
            )} />
            <TableColumn field="severity" header="Severity" sortable body={(event: AlarmEvent) => <StatusTag tone={severityTone[event.severity]}>{event.severity}</StatusTag>} />
            <TableColumn header="สถานะ" body={(event: AlarmEvent) => (
              event.acknowledgedBy ? (
                <StatusTag tone="active">Acked {event.acknowledgedAt ? formatDate(event.acknowledgedAt) : ""}</StatusTag>
              ) : (
                <Button variant="icon" onClick={() => void acknowledge(event)} title="Acknowledge" aria-label={`Acknowledge alarm ${event.id}`}><Check size={17} /></Button>
              )
            )} />
          </DataTable>
        )
      ) : tab === "logbook" ? (
        <div className="grid gap-4">
          <form className="alarm-logbook-form" onSubmit={(event) => void createLogbookEntry(event)}>
            <label>ประเภท
              <select className="ea-input" name="eventType" defaultValue="NOTE"><option value="FAULT">Fault</option><option value="MAINTENANCE">Maintenance</option><option value="CURTAILMENT">Curtailment</option><option value="NOTE">Note</option></select>
            </label>
            <label>Device (ถ้ามี)
              <select className="ea-input" name="deviceId" defaultValue=""><option value="">ทั้ง Plant</option>{devices.map((device) => <option key={device.id} value={device.id}>{device.name}</option>)}</select>
            </label>
            <label>หัวข้อ<TextInput name="title" required maxLength={200} placeholder="เช่น Inverter inspection" /></label>
            <label>เวลาเริ่ม<TextInput name="startsAt" type="datetime-local" required /></label>
            <label>หมวดหมู่<TextInput name="category" maxLength={100} placeholder="เช่น Preventive Maintenance" /></label>
            <label className="full-field">รายละเอียด<TextInput name="note" maxLength={4000} placeholder="บันทึกรายละเอียดเหตุการณ์" /></label>
            <Button type="submit" compact><Plus size={17} /> เพิ่ม Event</Button>
          </form>
          <DataTable value={logbook} dataKey="id" aria-label="Event Logbook" emptyMessage={<div className="table-state">ยังไม่มี Event Logbook</div>}>
            <TableColumn field="startsAt" header="เวลา" sortable body={(entry: EventLogbookEntry) => <div className="grid gap-1"><strong>{formatDate(entry.startsAt)}</strong>{entry.endsAt && <small>ถึง {formatDate(entry.endsAt)}</small>}</div>} />
            <TableColumn field="eventType" header="ประเภท" sortable body={(entry: EventLogbookEntry) => <StatusTag tone={entry.eventType === "FAULT" ? "offline" : entry.eventType === "MAINTENANCE" ? "degraded" : "active"}>{entry.eventType}</StatusTag>} />
            <TableColumn header="หัวข้อ" body={(entry: EventLogbookEntry) => <div className="grid gap-1"><strong>{entry.title}</strong><small className="text-ink-soft">{entry.category || "-"}{entry.deviceId ? ` · ${deviceName(entry.deviceId)}` : " · ทั้ง Plant"}</small></div>} />
            <TableColumn field="note" header="รายละเอียด" />
          </DataTable>
        </div>
      ) : (
        loading ? (
          <div className="table-state">กำลังโหลดข้อมูล</div>
        ) : (
          <DataTable value={rules} dataKey="id" aria-label="Alarm Rules" emptyMessage={<div className="table-state">{error ? "" : "ยังไม่มีกฎแจ้งเตือนสำหรับโรงไฟฟ้านี้"}</div>}>
            <TableColumn field="label" header="กฎ" sortable body={(rule: AlarmRule) => (
              <div className="grid gap-1"><strong>{rule.label}</strong><small className="block text-[11px] text-ink-soft">{rule.severity}{rule.alarmDelaySeconds > 0 ? ` · delay ${Math.round(rule.alarmDelaySeconds / 60)} นาที` : ""}{notifyRoleName(rule.notifyRoleId) ? ` · แจ้ง ${notifyRoleName(rule.notifyRoleId)}` : ""}</small></div>
            )} />
            <TableColumn header="Device / Point" body={(rule: AlarmRule) => (
              <div className="grid gap-1"><span>{deviceName(rule.deviceId)}</span><small className="block text-[11px] text-ink-soft">{conditionExpression((rule.conditions ?? []).map((c) => ({ pointKey: pointLabel(rule.deviceId, c.pointKey), logic: c.logic })))}</small></div>
            )} />
            <TableColumn header="Threshold" body={(rule: AlarmRule) => (
              <div className="grid gap-1">{(rule.conditions ?? []).map((c) => <div key={c.pointKey}><span>{pointLabel(rule.deviceId, c.pointKey)}</span> <small>{conditionThreshold(c)}</small></div>)}</div>
            )} />
            <TableColumn field="isActive" header="สถานะ" sortable body={(rule: AlarmRule) => (
              <StatusTag tone={rule.isActive ? "active" : "revoked"}>{rule.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>
            )} />
            <TableColumn header="" body={(rule: AlarmRule) => (
              <div className="row-actions">
                <Button variant="icon" onClick={() => setEditor(rule)} title="แก้ไขกฎ" aria-label={`แก้ไข ${rule.label}`}><Pencil size={17} /></Button>
                <Button variant="icon" danger onClick={() => void deleteRule(rule)} title="ลบกฎ" aria-label={`ลบ ${rule.label}`}><Trash2 size={17} /></Button>
              </div>
            )} />
          </DataTable>
        )
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

type ConditionDraft = { pointKey: string; minValue: string; maxValue: string; logic: ConditionLogic };

function draftFromCondition(condition: AlarmRuleCondition): ConditionDraft {
  return {
    pointKey: condition.pointKey,
    minValue: condition.minValue?.toString() ?? "",
    maxValue: condition.maxValue?.toString() ?? "",
    logic: condition.logic ?? "AND",
  };
}

const emptyCondition = (): ConditionDraft => ({ pointKey: "", minValue: "", maxValue: "", logic: "AND" });

/**
 * The rule as an expression, for the rules table. AND binds tighter than OR
 * (the backend folds the same way, see core.EvaluateConditions), so grouped
 * AND terms get brackets to make that visible rather than implied.
 */
function conditionExpression(conditions: Array<{ pointKey: string; logic?: ConditionLogic }>) {
  if (conditions.length === 0) return "-";
  const terms: string[][] = [[]];
  for (const [index, condition] of conditions.entries()) {
    if (index > 0 && condition.logic === "OR") terms.push([]);
    terms[terms.length - 1].push(condition.pointKey || "?");
  }
  return terms
    .map((term) => (terms.length > 1 && term.length > 1 ? `(${term.join(" และ ")})` : term.join(" และ ")))
    .join(" หรือ ");
}

function AlarmRuleEditor({ plantId, rule, devices, notifyRoles, onClose, onSaved }: { plantId: string; rule?: AlarmRule; devices: Device[]; notifyRoles: AlarmNotifyRole[]; onClose: () => void; onSaved: () => void }) {
  const [deviceId, setDeviceId] = useState(rule?.deviceId ?? devices[0]?.id ?? "");
  const [label, setLabel] = useState(rule?.label ?? "");
  const [conditions, setConditions] = useState<ConditionDraft[]>(
    rule && rule.conditions?.length > 0 ? rule.conditions.map(draftFromCondition) : [emptyCondition()],
  );
  const [severity, setSeverity] = useState<AlarmRule["severity"]>(rule?.severity ?? "warning");
  // Edited in minutes; stored in seconds. 0 = no delay, which is how every
  // existing rule behaves.
  const [alarmDelayMinutes, setAlarmDelayMinutes] = useState(String(Math.round((rule?.alarmDelaySeconds ?? 0) / 60)));
  const [isActive, setIsActive] = useState(rule?.isActive ?? true);
  const [notifyRoleId, setNotifyRoleId] = useState(rule?.notifyRoleId ?? "");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  // Register catalog for the rule's device, so Point key is a pick-list of
  // display names rather than an address the operator has to know by heart.
  const [catalog, setCatalog] = useState<Record<string, PointMeta>>({});
  const device = devices.find((entry) => entry.id === deviceId);

  useEffect(() => {
    setCatalog({});
    if (!plantId || !device) return;
    const controller = new AbortController();
    void loadRegisterCatalog(plantId, device, controller.signal)
      .then(setCatalog)
      // Falls back to the free-text input below, which is what this was before.
      .catch(() => undefined);
    return () => controller.abort();
  }, [plantId, device]);

  const points = useMemo(() => pointOptions(catalog), [catalog]);

  function optionalNumber(value: string) {
    return value.trim() === "" ? null : Number(value);
  }

  function updateCondition(index: number, patch: Partial<ConditionDraft>) {
    setConditions((current) => current.map((condition, i) => (i === index ? { ...condition, ...patch } : condition)));
  }

  function addCondition() {
    setConditions((current) => [...current, emptyCondition()]);
  }

  function removeCondition(index: number) {
    // Row 0 owns no connector, so if it is the one removed the new first row
    // has to give its own up -- otherwise a leading "OR" would survive and the
    // backend would silently normalize it away, disagreeing with the UI.
    setConditions((current) => current.filter((_, i) => i !== index)
      .map((condition, i) => (i === 0 ? { ...condition, logic: "AND" as ConditionLogic } : condition)));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const notifyRoleValue = notifyRoleId === "" ? null : notifyRoleId;
      const conditionsPayload = conditions.map((condition, index) => ({
        pointKey: condition.pointKey, minValue: optionalNumber(condition.minValue), maxValue: optionalNumber(condition.maxValue),
        logic: index === 0 ? "AND" : condition.logic,
      }));
      const alarmDelaySeconds = Math.max(0, Math.round(Number(alarmDelayMinutes) || 0)) * 60;
      const body = rule
        ? { label, conditions: conditionsPayload, severity, isActive, alarmDelaySeconds, notifyRoleId: notifyRoleValue }
        : { deviceId, label, conditions: conditionsPayload, severity, alarmDelaySeconds, notifyRoleId: notifyRoleValue };
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
            <label className="full-field">ชื่อกฎ<TextInput autoFocus value={label} onChange={(event) => setLabel(event.target.value)} maxLength={200} required /></label>

            <div className="full-field alarm-conditions">
              <div className="alarm-conditions-head">
                <span>Criteria (point + threshold) — เลือก AND/OR ของแต่ละเงื่อนไข</span>
              </div>
              {conditions.map((condition, index) => (
                <div className="alarm-condition-row" key={index}>
                  {/* Row 0 joins to nothing, so it shows a fixed "เมื่อ" label
                      instead of a connector it could never use. */}
                  {index === 0 ? (
                    <span className="alarm-condition-logic alarm-condition-logic-first">เมื่อ</span>
                  ) : (
                    <Select value={condition.logic} onValueChange={(value) => updateCondition(index, { logic: value as ConditionLogic })}>
                      <SelectTrigger className="alarm-condition-logic" aria-label={`Logic ก่อนเงื่อนไข ${index + 1}`}><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="AND">และ (AND)</SelectItem>
                        <SelectItem value="OR">หรือ (OR)</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                  {/* Falls back to the raw text input when the device has no
                      register metadata yet, so a rule can still be written. */}
                  {points.length > 0 ? (
                    <Select value={condition.pointKey} onValueChange={(value) => updateCondition(index, { pointKey: value })}>
                      <SelectTrigger aria-label={`Point ของเงื่อนไข ${index + 1}`}><SelectValue placeholder="เลือก Point" /></SelectTrigger>
                      <SelectContent>
                        {/* A rule saved against a register that has since been
                            renamed or disabled keeps its own value selectable,
                            instead of silently resetting on the next save. */}
                        {condition.pointKey && !points.some((point) => point.value === condition.pointKey) && (
                          <SelectItem value={condition.pointKey}>{condition.pointKey} (ไม่พบใน Metadata)</SelectItem>
                        )}
                        {points.map((point) => (
                          <SelectItem key={point.value} value={point.value}>
                            {point.label}{point.unit ? ` (${point.unit})` : ""} · {point.tag}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <TextInput placeholder="Point key" value={condition.pointKey} onChange={(event) => updateCondition(index, { pointKey: event.target.value })} maxLength={200} required />
                  )}
                  <TextInput type="number" step="any" placeholder="Min" value={condition.minValue} onChange={(event) => updateCondition(index, { minValue: event.target.value })} />
                  <TextInput type="number" step="any" placeholder="Max" value={condition.maxValue} onChange={(event) => updateCondition(index, { maxValue: event.target.value })} />
                  <Button type="button" variant="icon" danger disabled={conditions.length <= 1} onClick={() => removeCondition(index)} title="ลบเงื่อนไขนี้" aria-label={`ลบเงื่อนไข ${index + 1}`}><Trash2 size={16} /></Button>
                </div>
              ))}
              <Button type="button" variant="secondary" compact onClick={addCondition}><Plus size={15} /> เพิ่มเงื่อนไข</Button>
              {conditions.length > 1 && (
                <p className="alarm-conditions-preview">
                  แจ้งเตือนเมื่อ: <strong>{conditionExpression(conditions)}</strong> — AND มีลำดับก่อน OR
                </p>
              )}
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
            <label className="full-field">Alarm Delay (นาที)
              <TextInput type="number" min={0} max={1440} value={alarmDelayMinutes} onChange={(event) => setAlarmDelayMinutes(event.target.value)} />
              <small className="muted-text">หลังจากเกิด Alarm แล้ว ถ้าเกิดซ้ำภายในช่วงนี้จะไม่แจ้งเตือนและไม่บันทึกลง Log อีก · 0 = ปิด</small>
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
            {rule && <label className="toggle-field full-field"><Checkbox checked={isActive} onChange={setIsActive} /><span>เปิดใช้งานกฎนี้</span></label>}
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

"use client";

import { ArrowLeft, Download, Pencil, Plus, RefreshCw, Search, Trash2, Upload } from "lucide-react";
import { Button } from "../../components/ui/button";
import { Checkbox, FormMessage, TextArea, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { toast } from "../../components/ui/sonner";
import { iconButtonClass, inputClass, labelClass, primaryButtonClass, secondaryButtonClass } from "../../components/ui";
import { api, errorMessage, csrfToken, downloadBlob } from "../../lib/api";
import { MIDDLEWARE_DATA_TYPES, type DeviceModelOption, type DeviceModelRegisterMetadata, type RegisterProfile, type RegisterValueMapping } from "../../lib/types";
import { usePlatformSession } from "../../components/platform-shell";
import { can } from "../../lib/permissions";

export function RegisterMetadataPage() {
  const { user } = usePlatformSession();
  const [models, setModels] = useState<DeviceModelOption[]>([]);
  const [profiles, setProfiles] = useState<RegisterProfile[]>([]);
  const [selectedModelId, setSelectedModelId] = useState("");
  const [items, setItems] = useState<DeviceModelRegisterMetadata[]>([]);
  const [modelDialog, setModelDialog] = useState<DeviceModelOption | "create" | null>(null);
  const [addressDialog, setAddressDialog] = useState<DeviceModelRegisterMetadata | "create" | null>(null);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadModels = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/device-models");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู Device Model");
      if (!response.ok) throw new Error("ไม่สามารถโหลด Device Model ได้");
      setModels((await response.json()) as DeviceModelOption[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadProfiles = useCallback(async () => {
    const response = await api("/api/v1/register-profiles");
    if (response.ok) setProfiles((await response.json()) as RegisterProfile[]);
  }, []);

  const loadItems = useCallback(async (modelId: string) => {
    if (!modelId) {
      setItems([]);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const response = await api(`/api/v1/device-models/${encodeURIComponent(modelId)}/register-metadata`);
      if (!response.ok) throw new Error(response.status === 404 ? "ไม่พบ Device Model" : "ไม่สามารถโหลด Address Metadata ได้");
      setItems((await response.json()) as DeviceModelRegisterMetadata[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadModels(); }, [loadModels]);
  useEffect(() => { void loadProfiles(); }, [loadProfiles]);
  useEffect(() => { void loadItems(selectedModelId); }, [loadItems, selectedModelId]);

  const importInputRef = useRef<HTMLInputElement>(null);

  async function exportCSV() {
    const path = selectedModelId
      ? `/api/v1/device-models/${encodeURIComponent(selectedModelId)}/register-metadata/export`
      : "/api/v1/device-models/register-metadata/export-all";
    const response = await api(path);
    if (!response.ok) { toast.error("ดาวน์โหลด CSV ไม่สำเร็จ"); return; }
    downloadBlob(await response.blob(), "register-metadata.csv");
  }

  async function downloadTemplate() {
    const response = await api("/api/v1/device-models/register-metadata/import-template");
    if (!response.ok) { toast.error("ดาวน์โหลด Template ไม่สำเร็จ"); return; }
    downloadBlob(await response.blob(), "register-metadata-template.csv");
  }

  async function importCSVFile(file: File) {
    const formData = new FormData();
    formData.append("file", file);
    try {
      const response = await api("/api/v1/device-models/register-metadata/import", { method: "POST", headers: { "X-CSRF-Token": csrfToken() }, body: formData });
      if (!response.ok) throw new Error("Import CSV ไม่สำเร็จ — ตรวจรูปแบบไฟล์");
      const result = (await response.json()) as { deviceModelsCreated: number; rowsUpserted: number; rowsSkipped: number; errors?: string[] };
      toast.success(
        `Import สำเร็จ: Model ใหม่ ${result.deviceModelsCreated}, Register ${result.rowsUpserted} แถว`
        + (result.rowsSkipped > 0 ? `, ข้าม ${result.rowsSkipped} แถว (${(result.errors ?? []).slice(0, 2).join("; ")})` : ""),
      );
      await loadModels();
      if (selectedModelId) await loadItems(selectedModelId);
    } catch (cause) {
      toast.error(errorMessage(cause));
    }
  }

  const selectedModel = models.find((model) => model.id === selectedModelId);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filteredModels = useMemo(() => models.filter((model) =>
    !normalizedQuery || [model.manufacturer, model.deviceType, model.model, String(model.sourceTypeId ?? "")]
      .some((value) => value.toLocaleLowerCase().includes(normalizedQuery))
  ), [models, normalizedQuery]);
  const filteredItems = useMemo(() => items.filter((item) =>
    !normalizedQuery || [item.addressKey, item.displayName, item.unit, item.modbusDataType || item.dataType, item.notes]
      .some((value) => value.toLocaleLowerCase().includes(normalizedQuery))
  ), [items, normalizedQuery]);

  function modelSaved(model: DeviceModelOption) {
    setModels((current) => [model, ...current.filter((item) => item.id !== model.id)]
      .sort((a, b) => `${a.manufacturer} ${a.deviceType} ${a.model}`.localeCompare(`${b.manufacturer} ${b.deviceType} ${b.model}`)));
    setSelectedModelId(model.id);
    setModelDialog(null);
    setQuery("");
    toast.success(`บันทึก Model ${model.manufacturer} ${model.model} แล้ว`);
  }

  function addressSaved(item: DeviceModelRegisterMetadata) {
    setItems((current) => [item, ...current.filter((entry) => entry.addressKey !== item.addressKey)]
      .sort((a, b) => a.addressKey.localeCompare(b.addressKey)));
    setAddressDialog(null);
    toast.success(`บันทึก Address ${item.addressKey} แล้ว`);
  }

  async function removeAddress(item: DeviceModelRegisterMetadata) {
    if (!selectedModel || !window.confirm(`ลบ Address ${item.addressKey} จาก Model ${selectedModel.model}?`)) return;
    setError("");
    const response = await api(
      `/api/v1/device-models/${encodeURIComponent(selectedModel.id)}/register-metadata/${encodeURIComponent(item.addressKey)}`,
      { method: "DELETE", headers: { "X-CSRF-Token": csrfToken() } },
    );
    if (response.ok) {
      setItems((current) => current.filter((entry) => entry.addressKey !== item.addressKey));
      toast.success(`ลบ Address ${item.addressKey} แล้ว`);
    } else {
      setError(response.status === 404 ? "ไม่พบ Address Metadata" : "ไม่สามารถลบ Address Metadata ได้");
    }
  }

  async function assignProfile(profileId: string) {
    if (!selectedModel || profileId === selectedModel.registerProfileId) return;
    const response = await api(`/api/v1/device-models/${encodeURIComponent(selectedModel.id)}/register-profile`, {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ profileId }),
    });
    if (!response.ok) {
      setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยน Register Profile" : "ไม่สามารถเปลี่ยน Register Profile ได้");
      return;
    }
    setModels((current) => current.map((model) => model.id === selectedModel.id ? { ...model, registerProfileId: profileId } : model));
    await loadItems(selectedModel.id);
    toast.success("เปลี่ยน Register Profile แล้ว");
  }

  async function hardDeleteModel(model: DeviceModelOption) {
    const expected = "DELETE";
    if (window.prompt(`คำสั่งนี้จะลบ Model, Device ที่ใช้ Model นี้, Metadata และ normalized telemetry ถาวร\nพิมพ์ ${expected}`) !== expected) return;
    const response = await api(`/api/v1/device-models/${encodeURIComponent(model.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) {
      setSelectedModelId("");
      setItems([]);
      await loadModels();
      toast.success(`ลบ Model ${model.manufacturer} ${model.model} ถาวรแล้ว`);
    } else {
      setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ลบ Device Model ถาวรได้" : "ไม่สามารถลบ Device Model ถาวรได้");
    }
  }

  const rows = selectedModel ? filteredItems : filteredModels;

  return (
    <div className="content space-y-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div className="flex min-w-0 items-start gap-2">
          {selectedModel && (
            <Button variant="bare" className={iconButtonClass} type="button" onClick={() => { setSelectedModelId(""); setQuery(""); }} title="กลับไป Device Model" aria-label="กลับไป Device Model">
              <ArrowLeft size={18} />
            </Button>
          )}
          <div className="min-w-0">
            <p className="text-xs font-extrabold uppercase text-ink-soft">{selectedModel ? `${selectedModel.manufacturer} / ${selectedModel.deviceType}` : "Configuration registry"}</p>
            <h2 className="truncate text-2xl font-extrabold text-ink">{selectedModel ? selectedModel.model : "Device Models"}</h2>
          </div>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <label className="relative min-w-0 sm:w-72">
            <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" size={17} />
            <span className="sr-only">ค้นหา</span>
            <TextInput className={`${inputClass} pl-9`} type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={selectedModel ? "ค้นหา address, name, unit..." : "ค้นหา brand, type, model..."} />
          </label>
          <Button variant="bare" className={iconButtonClass} type="button" onClick={() => void (selectedModel ? loadItems(selectedModel.id) : loadModels())} title="รีเฟรช" aria-label="รีเฟรช">
            <RefreshCw size={18} />
          </Button>
          <Button variant="bare" className={secondaryButtonClass} type="button" onClick={() => void exportCSV()} title={selectedModel ? "Export CSV (Model นี้)" : "Export CSV (ทุก Model)"}>
            <Download size={16} /> Export CSV
          </Button>
          <Button variant="bare" className={secondaryButtonClass} type="button" onClick={() => void downloadTemplate()} title="ดาวน์โหลด Template เปล่า">
            Template
          </Button>
          <Button variant="bare" className={secondaryButtonClass} type="button" onClick={() => importInputRef.current?.click()} title="Import CSV (อัปเดตทับของเดิม)">
            <Upload size={16} /> Import CSV
          </Button>
          <input
            ref={importInputRef}
            type="file"
            accept=".csv,text/csv"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) void importCSVFile(file);
              event.target.value = "";
            }}
          />
          {can(user, "device_model", "create") && <Button variant="bare" className={primaryButtonClass} type="button" onClick={() => selectedModel ? setAddressDialog("create") : setModelDialog("create")}>
            <Plus size={17} /> {selectedModel ? "เพิ่ม Address" : "เพิ่ม Model"}
          </Button>}
        </div>
      </div>

      {selectedModel && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-y border-line py-3 text-sm">
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-ink-soft">
            <span>Brand <strong className="text-ink">{selectedModel.manufacturer}</strong></span>
            <span>ชนิด <strong className="text-ink">{selectedModel.deviceType}</strong></span>
            <span>Source Type <strong className="text-ink">{selectedModel.sourceTypeId ?? "-"}</strong></span>
            <label className="flex items-center gap-2">Register Profile
              <Select value={selectedModel.registerProfileId} onValueChange={(value) => void assignProfile(value)} disabled={!can(user, "device_model", "update")}>
                <SelectTrigger className="h-8 min-w-56"><SelectValue placeholder="เลือก Profile" /></SelectTrigger>
                <SelectContent>{profiles.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.name} {profile.manufacturer ? `· ${profile.manufacturer}` : ""}</SelectItem>)}</SelectContent>
              </Select>
            </label>
          </div>
          <div className="flex flex-wrap gap-2">
            {can(user, "device_model", "update") && <Button variant="bare" className={secondaryButtonClass} type="button" onClick={() => setModelDialog(selectedModel)}><Pencil size={16} /> แก้ไข Model</Button>}
            {can(user, "device_model", "hard_delete") && <Button variant="bare" className={`${secondaryButtonClass} text-danger`} type="button" onClick={() => void hardDeleteModel(selectedModel)}><Trash2 size={16} /> ลบถาวร</Button>}
          </div>
        </div>
      )}

      {error && <FormMessage>{error}</FormMessage>}

      <section className="overflow-hidden rounded-md border border-line bg-white" aria-label={selectedModel ? "Address Metadata" : "Device Models"}>
        {selectedModel ? (
          <>
            <div className="hidden min-h-11 grid-cols-[minmax(130px,1fr)_minmax(150px,1.2fr)_80px_100px_150px_80px_96px] items-center gap-3 border-b border-line bg-canvas px-4 text-xs font-extrabold text-ink-soft lg:grid">
              <span>Address</span><span>Display name</span><span>Unit</span><span>Type</span><span>Transform</span><span>Status</span><span aria-label="คำสั่ง" />
            </div>
            {filteredItems.map((item) => (
              <div key={item.addressKey} className="grid grid-cols-[minmax(0,1fr)_88px] gap-3 border-b border-line px-4 py-3 text-sm last:border-b-0 lg:min-h-16 lg:grid-cols-[minmax(130px,1fr)_minmax(150px,1.2fr)_80px_100px_150px_80px_96px] lg:items-center">
                <div className="min-w-0"><strong className="block truncate text-ink">{item.addressKey}</strong><small className="block truncate text-xs text-ink-soft lg:hidden">{item.displayName || "ยังไม่ระบุชื่อ"} · {item.unit || "ไม่มีหน่วย"}</small></div>
                <span className="hidden truncate text-ink lg:block">{item.displayName || "-"}</span>
                <span className="hidden truncate text-ink lg:block">{item.unit || "-"}</span>
                <span className="hidden truncate text-ink lg:block">{item.modbusDataType || item.dataType}</span>
                <span className="hidden truncate text-ink lg:block">x{item.scale} + {item.offset}, {item.decimals} dp</span>
                <span className={`hidden w-fit rounded-full px-2.5 py-1 text-xs font-extrabold lg:block ${item.isEnabled ? "bg-success/10 text-success" : "bg-danger/10 text-danger"}`}>{item.isEnabled ? "เปิด" : "ปิด"}</span>
                <div className="row-start-1 flex justify-end gap-1 lg:row-auto">
                  {can(user, "device_model", "update") && <Button variant="bare" className={iconButtonClass} type="button" onClick={() => setAddressDialog(item)} title="แก้ไข Address" aria-label={`แก้ไข ${item.addressKey}`}><Pencil size={16} /></Button>}
                  {can(user, "device_model", "update") && <Button variant="bare" className={`${iconButtonClass} text-danger hover:border-danger/30 hover:bg-danger/10`} type="button" onClick={() => void removeAddress(item)} title="ลบ Address" aria-label={`ลบ ${item.addressKey}`}><Trash2 size={16} /></Button>}
                </div>
              </div>
            ))}
          </>
        ) : (
          <>
            <div className="hidden min-h-11 grid-cols-[minmax(150px,1fr)_minmax(120px,.8fr)_minmax(180px,1.2fr)_100px_90px_96px] items-center gap-3 border-b border-line bg-canvas px-4 text-xs font-extrabold text-ink-soft md:grid">
              <span>Brand</span><span>ชนิด</span><span>รุ่น</span><span>Source</span><span>Status</span><span aria-label="คำสั่ง" />
            </div>
            {filteredModels.map((model) => (
              <div key={model.id} role="button" tabIndex={0} className="grid cursor-pointer grid-cols-[minmax(0,1fr)_88px] gap-3 border-b border-line px-4 py-3 text-sm transition last:border-b-0 hover:bg-canvas focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-focus md:min-h-16 md:grid-cols-[minmax(150px,1fr)_minmax(120px,.8fr)_minmax(180px,1.2fr)_100px_90px_96px] md:items-center" onClick={() => { setSelectedModelId(model.id); setQuery(""); }} onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  setSelectedModelId(model.id);
                  setQuery("");
                }
              }}>
                <div className="min-w-0"><strong className="block truncate text-ink">{model.manufacturer}</strong><small className="block truncate text-xs text-ink-soft md:hidden">{model.deviceType} · {model.model}</small></div>
                <span className="hidden truncate text-ink md:block">{model.deviceType}</span>
                <span className="hidden truncate text-ink md:block">{model.model}</span>
                <span className="hidden truncate text-ink md:block">{model.sourceTypeId ?? "-"}</span>
                <span className={`hidden w-fit rounded-full px-2.5 py-1 text-xs font-extrabold md:block ${model.isActive ? "bg-success/10 text-success" : "bg-danger/10 text-danger"}`}>{model.isActive ? "เปิด" : "ปิด"}</span>
                <div className="flex justify-end gap-1">
                  {can(user, "device_model", "update") && <Button variant="bare" className={iconButtonClass} type="button" onClick={(event) => { event.stopPropagation(); setModelDialog(model); }} title="แก้ไข Model" aria-label={`แก้ไข ${model.model}`}><Pencil size={16} /></Button>}
                  {can(user, "device_model", "hard_delete") && <Button variant="bare" className={`${iconButtonClass} text-danger`} type="button" onClick={(event) => { event.stopPropagation(); void hardDeleteModel(model); }} title="ลบ Model ถาวร" aria-label={`ลบ ${model.model} ถาวร`}><Trash2 size={16} /></Button>}
                </div>
              </div>
            ))}
          </>
        )}
        {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
        {!loading && rows.length === 0 && <div className="table-state">{query ? "ไม่พบข้อมูลที่ตรงกับคำค้น" : selectedModel ? "ยังไม่มี Address Metadata สำหรับ Model นี้" : "ยังไม่มี Device Model"}</div>}
      </section>

      {modelDialog && <DeviceModelDialog model={modelDialog === "create" ? null : modelDialog} models={models} onClose={() => setModelDialog(null)} onSaved={modelSaved} />}
      {addressDialog && selectedModel && <AddressMetadataDialog model={selectedModel} item={addressDialog === "create" ? null : addressDialog} onClose={() => setAddressDialog(null)} onSaved={addressSaved} />}
    </div>
  );
}

const NEW_DEVICE_TYPE = "__new__";

function DeviceModelDialog({ model, models, onClose, onSaved }: { model: DeviceModelOption | null; models: DeviceModelOption[]; onClose: () => void; onSaved: (model: DeviceModelOption) => void }) {
  const [manufacturer, setManufacturer] = useState(model?.manufacturer ?? "");
  const [deviceType, setDeviceType] = useState(model?.deviceType ?? "");
  const [modelName, setModelName] = useState(model?.model ?? "");
  const [sourceTypeId, setSourceTypeId] = useState(model?.sourceTypeId == null ? "" : String(model.sourceTypeId));
  const [isActive, setIsActive] = useState(model?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const deviceTypeOptions = useMemo(() => {
    const bySourceType = new Map<string, number | null>();
    for (const item of models) if (!bySourceType.has(item.deviceType)) bySourceType.set(item.deviceType, item.sourceTypeId ?? null);
    return [...bySourceType.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [models]);
  const [customDeviceType, setCustomDeviceType] = useState(!deviceTypeOptions.some(([type]) => type === deviceType));

  function selectDeviceType(value: string) {
    if (value === NEW_DEVICE_TYPE) {
      setCustomDeviceType(true);
      setDeviceType("");
      setSourceTypeId("");
      return;
    }
    setCustomDeviceType(false);
    setDeviceType(value);
    const mapped = deviceTypeOptions.find(([type]) => type === value)?.[1];
    setSourceTypeId(mapped == null ? "" : String(mapped));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api(model ? `/api/v1/device-models/${encodeURIComponent(model.id)}` : "/api/v1/device-models", {
        method: model ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ manufacturer, deviceType, model: modelName, sourceTypeId: sourceTypeId === "" ? null : Number(sourceTypeId), isActive }),
      });
      if (!response.ok) throw new Error(response.status === 409 ? "Brand/ชนิด/รุ่นนี้มีอยู่แล้ว" : "ไม่สามารถบันทึก Device Model ได้");
      onSaved((await response.json()) as DeviceModelOption);
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
          <div><DialogDescription>Model configuration</DialogDescription><DialogTitle>{model ? "แก้ไข Device Model" : "เพิ่ม Device Model"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
            <label className={labelClass}>Brand<TextInput className={inputClass} autoFocus value={manufacturer} onChange={(event) => setManufacturer(event.target.value)} maxLength={200} required /></label>
            <label className={labelClass}>
              ชนิดอุปกรณ์
              {customDeviceType
                ? (
                  <span className="flex gap-2">
                    <TextInput className={inputClass} value={deviceType} onChange={(event) => setDeviceType(event.target.value)} maxLength={100} placeholder="ชนิดอุปกรณ์ใหม่" required autoFocus />
                    {deviceTypeOptions.length > 0 && <Button variant="bare" type="button" className={`${secondaryButtonClass} shrink-0 text-xs`} onClick={() => selectDeviceType(deviceTypeOptions[0][0])}>เลือกจากรายการ</Button>}
                  </span>
                )
                : (
                  <Select value={deviceType} onValueChange={selectDeviceType}>
                    <SelectTrigger aria-label="ชนิดอุปกรณ์"><SelectValue placeholder="เลือกชนิดอุปกรณ์" /></SelectTrigger>
                    <SelectContent>
                      {deviceTypeOptions.map(([type]) => <SelectItem key={type} value={type}>{type}</SelectItem>)}
                      <SelectItem value={NEW_DEVICE_TYPE}>+ เพิ่มชนิดใหม่</SelectItem>
                    </SelectContent>
                  </Select>
                )}
            </label>
            <label className={`${labelClass} sm:col-span-2`}>รุ่น<TextInput className={inputClass} value={modelName} onChange={(event) => setModelName(event.target.value)} maxLength={200} required /></label>
            <label className={labelClass}>
              Source Type ID
              <TextInput className={inputClass} type="number" min="0" value={sourceTypeId} onChange={(event) => setSourceTypeId(event.target.value)} readOnly={!customDeviceType && deviceTypeOptions.some(([type]) => type === deviceType)} />
            </label>
            <label className="flex items-center gap-2 self-end text-sm font-bold text-slate-800"><Checkbox checked={isActive} onChange={setIsActive} /> เปิดใช้งาน</label>
            {error && <FormMessage className="sm:col-span-2">{error}</FormMessage>}
            <div className="flex justify-end gap-2 sm:col-span-2"><Button variant="bare" type="button" className={secondaryButtonClass} onClick={onClose} disabled={pending}>ยกเลิก</Button><Button variant="bare" className={primaryButtonClass} disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function AddressMetadataDialog({ model, item, onClose, onSaved }: { model: DeviceModelOption; item: DeviceModelRegisterMetadata | null; onClose: () => void; onSaved: (item: DeviceModelRegisterMetadata) => void }) {
  const [addressKey, setAddressKey] = useState(item?.addressKey ?? "");
  const [addressKeyEdited, setAddressKeyEdited] = useState(Boolean(item));
  const [displayName, setDisplayName] = useState(item?.displayName ?? "");
  const [unit, setUnit] = useState(item?.unit ?? "");
  const [dataType, setDataType] = useState<DeviceModelRegisterMetadata["dataType"]>(item?.dataType ?? "number");
  const [scale, setScale] = useState(String(item?.scale ?? 1));
  const [offset, setOffset] = useState(String(item?.offset ?? 0));
  const [decimals, setDecimals] = useState(String(item?.decimals ?? 2));
  const [isEnabled, setIsEnabled] = useState(item?.isEnabled ?? true);
  const [notes, setNotes] = useState(item?.notes ?? "");
  const [modbusFunctionCode, setModbusFunctionCode] = useState(item?.modbusFunctionCode == null ? "" : String(item.modbusFunctionCode));
  const [modbusRegister, setModbusRegister] = useState(item?.modbusRegister == null ? "" : String(item.modbusRegister));
  const [modbusWordOrder, setModbusWordOrder] = useState(item?.modbusWordOrder ?? "");
  const [modbusDataType, setModbusDataType] = useState(item?.modbusDataType ?? "");
  const [isAlarm, setIsAlarm] = useState(item?.isAlarm ?? false);
  const [mappingMode, setMappingMode] = useState<"EXACT" | "BITMASK">(item?.mappingMode ?? "EXACT");
  const [bitInterpretation, setBitInterpretation] = useState<"ONE_HOT" | "INDEPENDENT_FLAGS">(item?.bitInterpretation ?? "INDEPENDENT_FLAGS");
  const [mappings, setMappings] = useState<Array<RegisterValueMapping & { matchValueText: string; bitIndexText: string }>>(() => (item?.mappings ?? []).map((mapping) => ({ ...mapping, matchValueText: mapping.matchValue == null ? "" : String(mapping.matchValue), bitIndexText: mapping.bitIndex == null ? "" : String(mapping.bitIndex) })));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api(`/api/v1/device-models/${encodeURIComponent(model.id)}/register-metadata`, {
        method: "PUT",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({
          addressKey, displayName, unit, dataType, scale: Number(scale), offset: Number(offset), decimals: Number(decimals), isEnabled, notes,
          modbusFunctionCode: modbusFunctionCode === "" ? null : Number(modbusFunctionCode),
          modbusRegister: modbusRegister === "" ? null : Number(modbusRegister),
          modbusWordOrder, modbusDataType,
          isAlarm, mappingMode, bitInterpretation,
          mappings: mappings.map((mapping) => ({
            displayValue: mapping.displayValue,
            matchValue: mappingMode === "EXACT" && mapping.matchValueText !== "" ? Number(mapping.matchValueText) : null,
            bitIndex: mappingMode === "BITMASK" && mapping.bitIndexText !== "" ? Number(mapping.bitIndexText) : null,
            alarmState: isAlarm ? (mapping.alarmState || null) : null,
            severity: isAlarm ? (mapping.severity || null) : null,
          })),
        }),
      });
      if (!response.ok) throw new Error(response.status === 404 ? "ไม่พบ Device Model" : "Address Metadata ไม่ถูกต้องหรือไม่สามารถบันทึกได้");
      onSaved((await response.json()) as DeviceModelRegisterMetadata);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  useEffect(() => {
    if (item || addressKeyEdited) return;
    if (modbusFunctionCode === "" || modbusRegister === "") return;
    setAddressKey(`${modbusFunctionCode}:${modbusRegister}`);
  }, [item, addressKeyEdited, modbusFunctionCode, modbusRegister]);

  useEffect(() => {
    if (modbusDataType !== "") setDataType("number");
  }, [modbusDataType]);

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>{`${model.manufacturer} / ${model.model}`}</DialogDescription><DialogTitle>{item ? `แก้ไข ${item.addressKey}` : "เพิ่ม Address Metadata"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
            <label className={`${labelClass} sm:col-span-2`}>Address / Key<TextInput className={inputClass} autoFocus value={addressKey} onChange={(event) => { setAddressKey(event.target.value); setAddressKeyEdited(true); }} maxLength={200} readOnly={Boolean(item)} required /></label>

            <p className="sm:col-span-2 text-xs font-extrabold uppercase text-ink-soft">Modbus Register</p>
            <div className="sm:col-span-2 grid grid-cols-2 gap-4 sm:grid-cols-4">
              <Select value={modbusFunctionCode} onValueChange={setModbusFunctionCode}>
                <SelectTrigger><SelectValue placeholder="-" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="">-</SelectItem>
                  <SelectItem value="3">FC03</SelectItem>
                  <SelectItem value="4">FC04</SelectItem>
                </SelectContent>
              </Select>
              <TextInput className={inputClass} type="number" min="0" max="65535" placeholder="Register" value={modbusRegister} onChange={(event) => setModbusRegister(event.target.value)} />
              <Select value={modbusWordOrder} onValueChange={setModbusWordOrder}>
                <SelectTrigger><SelectValue placeholder="Word order (default)" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="">Word order (default)</SelectItem>
                  <SelectItem value="HIGH_LOW">HIGH_LOW</SelectItem>
                  <SelectItem value="LOW_HIGH">LOW_HIGH</SelectItem>
                </SelectContent>
              </Select>
              <Select value={modbusDataType} onValueChange={setModbusDataType}>
                <SelectTrigger><SelectValue placeholder="Modbus type" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="">Modbus type</SelectItem>
                  {MIDDLEWARE_DATA_TYPES.map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <p className="sm:col-span-2 -mb-2 text-xs text-ink-soft">เว้นว่างทั้งหมดถ้าเป็น display metadata อย่างเดียว ไม่ใช้ poll จริง</p>

            <p className="sm:col-span-2 text-xs font-extrabold uppercase text-ink-soft">Display</p>
            <label className={labelClass}>Display name<TextInput className={inputClass} value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} placeholder="Active power" /></label>
            <label className={labelClass}>Unit<TextInput className={inputClass} value={unit} onChange={(event) => setUnit(event.target.value)} maxLength={40} placeholder="kW" /></label>
            {modbusDataType === "" && (
              <label className={labelClass}>Data type
                <Select value={dataType} onValueChange={(value) => setDataType(value as DeviceModelRegisterMetadata["dataType"])}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="number">number</SelectItem>
                    <SelectItem value="boolean">boolean</SelectItem>
                    <SelectItem value="text">text</SelectItem>
                    <SelectItem value="enum">enum</SelectItem>
                  </SelectContent>
                </Select>
              </label>
            )}
            <label className={labelClass}>Scale<TextInput className={inputClass} type="number" step="any" value={scale} onChange={(event) => setScale(event.target.value)} required /></label>
            <label className={labelClass}>Offset<TextInput className={inputClass} type="number" step="any" value={offset} onChange={(event) => setOffset(event.target.value)} required /></label>
            <label className={labelClass}>Decimals<TextInput className={inputClass} type="number" min="0" max="9" value={decimals} onChange={(event) => setDecimals(event.target.value)} required /></label>
            <label className="flex items-center gap-2 self-end text-sm font-bold text-slate-800"><Checkbox checked={isEnabled} onChange={setIsEnabled} /> เปิดใช้งาน</label>

            <div className="sm:col-span-2 rounded-md border border-line bg-canvas/50 p-3">
              <div className="flex items-center justify-between gap-3">
                <label className="flex items-center gap-2 text-sm font-bold text-slate-800"><Checkbox checked={isAlarm} onChange={setIsAlarm} /> Address นี้เป็น Alarm</label>
                <Select value={mappingMode} onValueChange={(value) => setMappingMode(value as "EXACT" | "BITMASK")}>
                  <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem value="EXACT">Exact value</SelectItem><SelectItem value="BITMASK">Bit mapping</SelectItem></SelectContent>
                </Select>
              </div>
              {mappingMode === "BITMASK" && <div className="mt-3 grid gap-2 sm:grid-cols-2">
                <label className={labelClass}>Bit interpretation
                  <Select value={bitInterpretation} onValueChange={(value) => setBitInterpretation(value as "ONE_HOT" | "INDEPENDENT_FLAGS")}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem value="ONE_HOT">One-hot / single status</SelectItem><SelectItem value="INDEPENDENT_FLAGS">Independent flags</SelectItem></SelectContent>
                  </Select>
                </label>
                <p className="self-end text-xs text-ink-soft">One-hot ใช้เมื่อมีสถานะหลักเพียงหนึ่ง Bit; Independent ใช้เมื่อหลาย Bit เป็น 1 ได้พร้อมกัน</p>
              </div>}
              <div className="mt-3 space-y-2">
                {mappings.map((mapping, index) => <div key={index} className="grid gap-2 sm:grid-cols-[90px_90px_minmax(0,1fr)_auto]">
                  <TextInput className={inputClass} type="number" placeholder={mappingMode === "EXACT" ? "Value" : "Bit"} value={mappingMode === "EXACT" ? mapping.matchValueText : mapping.bitIndexText} onChange={(event) => setMappings((current) => current.map((entry, row) => row === index ? { ...entry, ...(mappingMode === "EXACT" ? { matchValueText: event.target.value } : { bitIndexText: event.target.value }) } : entry))} />
                  {isAlarm ? <Select value={mapping.severity ?? "warning"} onValueChange={(value) => setMappings((current) => current.map((entry, row) => row === index ? { ...entry, severity: value } : entry))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="info">Info</SelectItem><SelectItem value="warning">Warning</SelectItem><SelectItem value="critical">Critical</SelectItem></SelectContent></Select> : <span />}
                  <TextInput className={inputClass} placeholder="Display value เช่น Model A" value={mapping.displayValue} onChange={(event) => setMappings((current) => current.map((entry, row) => row === index ? { ...entry, displayValue: event.target.value } : entry))} />
                  <Button variant="icon" type="button" danger onClick={() => setMappings((current) => current.filter((_, row) => row !== index))} aria-label="ลบ mapping"><Trash2 size={16} /></Button>
                </div>)}
                <Button variant="bare" type="button" className={secondaryButtonClass} onClick={() => setMappings((current) => [...current, { matchValueText: "", bitIndexText: "", displayValue: "", severity: isAlarm ? "warning" : null }])}><Plus size={15} /> เพิ่ม Mapping</Button>
              </div>
            </div>

            <label className={`${labelClass} sm:col-span-2`}>Notes<TextArea className={`${inputClass} min-h-24 py-2`} value={notes} onChange={(event) => setNotes(event.target.value)} maxLength={500} /></label>
            {error && <FormMessage className="sm:col-span-2">{error}</FormMessage>}
            <div className="flex justify-end gap-2 sm:col-span-2"><Button variant="bare" type="button" className={secondaryButtonClass} onClick={onClose} disabled={pending}>ยกเลิก</Button><Button variant="bare" className={primaryButtonClass} disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก Address"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

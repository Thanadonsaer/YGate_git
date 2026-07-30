"use client";

import { ArrowLeft, Pencil, Plus, RefreshCw, Search, Trash2, X } from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { iconButtonClass, inputClass, labelClass, primaryButtonClass, secondaryButtonClass } from "../../components/ui";
import { api, csrfToken } from "../../lib/api";
import type { DeviceModelOption, RegisterMetadata } from "../../lib/types";

export function RegisterMetadataPage() {
  const [models, setModels] = useState<DeviceModelOption[]>([]);
  const [selectedModelId, setSelectedModelId] = useState("");
  const [items, setItems] = useState<RegisterMetadata[]>([]);
  const [modelDialog, setModelDialog] = useState<DeviceModelOption | "create" | null>(null);
  const [addressDialog, setAddressDialog] = useState<RegisterMetadata | "create" | null>(null);
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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
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
      setItems((await response.json()) as RegisterMetadata[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadModels(); }, [loadModels]);
  useEffect(() => { void loadItems(selectedModelId); }, [loadItems, selectedModelId]);

  const selectedModel = models.find((model) => model.id === selectedModelId);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filteredModels = useMemo(() => models.filter((model) =>
    !normalizedQuery || [model.manufacturer, model.deviceType, model.model, String(model.sourceTypeId ?? "")]
      .some((value) => value.toLocaleLowerCase().includes(normalizedQuery))
  ), [models, normalizedQuery]);
  const filteredItems = useMemo(() => items.filter((item) =>
    !normalizedQuery || [item.addressKey, item.displayName, item.unit, item.dataType, item.notes]
      .some((value) => value.toLocaleLowerCase().includes(normalizedQuery))
  ), [items, normalizedQuery]);

  function modelSaved(model: DeviceModelOption) {
    setModels((current) => [model, ...current.filter((item) => item.id !== model.id)]
      .sort((a, b) => `${a.manufacturer} ${a.deviceType} ${a.model}`.localeCompare(`${b.manufacturer} ${b.deviceType} ${b.model}`)));
    setSelectedModelId(model.id);
    setModelDialog(null);
    setQuery("");
  }

  function addressSaved(item: RegisterMetadata) {
    setItems((current) => [item, ...current.filter((entry) => entry.addressKey !== item.addressKey)]
      .sort((a, b) => a.addressKey.localeCompare(b.addressKey)));
    setAddressDialog(null);
  }

  async function removeAddress(item: RegisterMetadata) {
    if (!selectedModel || !window.confirm(`ลบ Address ${item.addressKey} จาก Model ${selectedModel.model}?`)) return;
    setError("");
    const response = await api(
      `/api/v1/device-models/${encodeURIComponent(selectedModel.id)}/register-metadata/${encodeURIComponent(item.addressKey)}`,
      { method: "DELETE", headers: { "X-CSRF-Token": csrfToken() } },
    );
    if (response.ok) {
      setItems((current) => current.filter((entry) => entry.addressKey !== item.addressKey));
    } else {
      setError(response.status === 404 ? "ไม่พบ Address Metadata" : "ไม่สามารถลบ Address Metadata ได้");
    }
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
            <button className={iconButtonClass} type="button" onClick={() => { setSelectedModelId(""); setQuery(""); }} title="กลับไป Device Model" aria-label="กลับไป Device Model">
              <ArrowLeft size={18} />
            </button>
          )}
          <div className="min-w-0">
            <p className="text-xs font-extrabold uppercase text-slate-500">{selectedModel ? `${selectedModel.manufacturer} / ${selectedModel.deviceType}` : "Configuration registry"}</p>
            <h2 className="truncate text-2xl font-extrabold text-slate-900">{selectedModel ? selectedModel.model : "Device Models"}</h2>
          </div>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <label className="relative min-w-0 sm:w-72">
            <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" size={17} />
            <span className="sr-only">ค้นหา</span>
            <input className={`${inputClass} pl-9`} type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={selectedModel ? "ค้นหา address, name, unit..." : "ค้นหา brand, type, model..."} />
          </label>
          <button className={iconButtonClass} type="button" onClick={() => void (selectedModel ? loadItems(selectedModel.id) : loadModels())} title="รีเฟรช" aria-label="รีเฟรช">
            <RefreshCw size={18} />
          </button>
          <button className={primaryButtonClass} type="button" onClick={() => selectedModel ? setAddressDialog("create") : setModelDialog("create")}>
            <Plus size={17} /> {selectedModel ? "เพิ่ม Address" : "เพิ่ม Model"}
          </button>
        </div>
      </div>

      {selectedModel && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-y border-slate-200 py-3 text-sm">
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-slate-600">
            <span>Brand <strong className="text-slate-900">{selectedModel.manufacturer}</strong></span>
            <span>ชนิด <strong className="text-slate-900">{selectedModel.deviceType}</strong></span>
            <span>Source Type <strong className="text-slate-900">{selectedModel.sourceTypeId ?? "-"}</strong></span>
          </div>
          <div className="flex flex-wrap gap-2">
            <button className={secondaryButtonClass} type="button" onClick={() => setModelDialog(selectedModel)}><Pencil size={16} /> แก้ไข Model</button>
            <button className={`${secondaryButtonClass} text-rose-700`} type="button" onClick={() => void hardDeleteModel(selectedModel)}><Trash2 size={16} /> ลบถาวร</button>
          </div>
        </div>
      )}

      {error && <p className="rounded-md bg-rose-50 px-3 py-2 text-sm font-bold text-rose-700">{error}</p>}

      <section className="overflow-hidden rounded-md border border-slate-200 bg-white" aria-label={selectedModel ? "Address Metadata" : "Device Models"}>
        {selectedModel ? (
          <>
            <div className="hidden min-h-11 grid-cols-[minmax(130px,1fr)_minmax(150px,1.2fr)_80px_100px_150px_80px_96px] items-center gap-3 border-b border-slate-200 bg-slate-50 px-4 text-xs font-extrabold text-slate-500 lg:grid">
              <span>Address</span><span>Display name</span><span>Unit</span><span>Type</span><span>Transform</span><span>Status</span><span aria-label="คำสั่ง" />
            </div>
            {filteredItems.map((item) => (
              <div key={item.addressKey} className="grid grid-cols-[minmax(0,1fr)_88px] gap-3 border-b border-slate-200 px-4 py-3 text-sm last:border-b-0 lg:min-h-16 lg:grid-cols-[minmax(130px,1fr)_minmax(150px,1.2fr)_80px_100px_150px_80px_96px] lg:items-center">
                <div className="min-w-0"><strong className="block truncate text-slate-900">{item.addressKey}</strong><small className="block truncate text-xs text-slate-500 lg:hidden">{item.displayName || "ยังไม่ระบุชื่อ"} · {item.unit || "ไม่มีหน่วย"}</small></div>
                <span className="hidden truncate text-slate-700 lg:block">{item.displayName || "-"}</span>
                <span className="hidden truncate text-slate-700 lg:block">{item.unit || "-"}</span>
                <span className="hidden truncate text-slate-700 lg:block">{item.dataType}</span>
                <span className="hidden truncate text-slate-700 lg:block">x{item.scale} + {item.offset}, {item.decimals} dp</span>
                <span className={`hidden w-fit rounded-full px-2.5 py-1 text-xs font-extrabold lg:block ${item.isEnabled ? "bg-emerald-50 text-emerald-700" : "bg-rose-50 text-rose-700"}`}>{item.isEnabled ? "เปิด" : "ปิด"}</span>
                <div className="row-start-1 flex justify-end gap-1 lg:row-auto">
                  <button className={iconButtonClass} type="button" onClick={() => setAddressDialog(item)} title="แก้ไข Address" aria-label={`แก้ไข ${item.addressKey}`}><Pencil size={16} /></button>
                  <button className={`${iconButtonClass} text-rose-700 hover:border-rose-200 hover:bg-rose-50`} type="button" onClick={() => void removeAddress(item)} title="ลบ Address" aria-label={`ลบ ${item.addressKey}`}><Trash2 size={16} /></button>
                </div>
              </div>
            ))}
          </>
        ) : (
          <>
            <div className="hidden min-h-11 grid-cols-[minmax(150px,1fr)_minmax(120px,.8fr)_minmax(180px,1.2fr)_100px_90px_96px] items-center gap-3 border-b border-slate-200 bg-slate-50 px-4 text-xs font-extrabold text-slate-500 md:grid">
              <span>Brand</span><span>ชนิด</span><span>รุ่น</span><span>Source</span><span>Status</span><span aria-label="คำสั่ง" />
            </div>
            {filteredModels.map((model) => (
              <div key={model.id} role="button" tabIndex={0} className="grid cursor-pointer grid-cols-[minmax(0,1fr)_88px] gap-3 border-b border-slate-200 px-4 py-3 text-sm transition last:border-b-0 hover:bg-slate-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-cyan-700 md:min-h-16 md:grid-cols-[minmax(150px,1fr)_minmax(120px,.8fr)_minmax(180px,1.2fr)_100px_90px_96px] md:items-center" onClick={() => { setSelectedModelId(model.id); setQuery(""); }} onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  setSelectedModelId(model.id);
                  setQuery("");
                }
              }}>
                <div className="min-w-0"><strong className="block truncate text-slate-900">{model.manufacturer}</strong><small className="block truncate text-xs text-slate-500 md:hidden">{model.deviceType} · {model.model}</small></div>
                <span className="hidden truncate text-slate-700 md:block">{model.deviceType}</span>
                <span className="hidden truncate text-slate-700 md:block">{model.model}</span>
                <span className="hidden truncate text-slate-700 md:block">{model.sourceTypeId ?? "-"}</span>
                <span className={`hidden w-fit rounded-full px-2.5 py-1 text-xs font-extrabold md:block ${model.isActive ? "bg-emerald-50 text-emerald-700" : "bg-rose-50 text-rose-700"}`}>{model.isActive ? "เปิด" : "ปิด"}</span>
                <div className="flex justify-end gap-1">
                  <button className={iconButtonClass} type="button" onClick={(event) => { event.stopPropagation(); setModelDialog(model); }} title="แก้ไข Model" aria-label={`แก้ไข ${model.model}`}><Pencil size={16} /></button>
                  <button className={`${iconButtonClass} text-rose-700`} type="button" onClick={(event) => { event.stopPropagation(); void hardDeleteModel(model); }} title="ลบ Model ถาวร" aria-label={`ลบ ${model.model} ถาวร`}><Trash2 size={16} /></button>
                </div>
              </div>
            ))}
          </>
        )}
        {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
        {!loading && rows.length === 0 && <div className="table-state">{query ? "ไม่พบข้อมูลที่ตรงกับคำค้น" : selectedModel ? "ยังไม่มี Address Metadata สำหรับ Model นี้" : "ยังไม่มี Device Model"}</div>}
      </section>

      {modelDialog && <DeviceModelDialog model={modelDialog === "create" ? null : modelDialog} onClose={() => setModelDialog(null)} onSaved={modelSaved} />}
      {addressDialog && selectedModel && <AddressMetadataDialog model={selectedModel} item={addressDialog === "create" ? null : addressDialog} onClose={() => setAddressDialog(null)} onSaved={addressSaved} />}
    </div>
  );
}

function DeviceModelDialog({ model, onClose, onSaved }: { model: DeviceModelOption | null; onClose: () => void; onSaved: (model: DeviceModelOption) => void }) {
  const [manufacturer, setManufacturer] = useState(model?.manufacturer ?? "");
  const [deviceType, setDeviceType] = useState(model?.deviceType ?? "");
  const [modelName, setModelName] = useState(model?.model ?? "");
  const [sourceTypeId, setSourceTypeId] = useState(model?.sourceTypeId == null ? "" : String(model.sourceTypeId));
  const [isActive, setIsActive] = useState(model?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog title={model ? "แก้ไข Device Model" : "เพิ่ม Device Model"} eyebrow="Model configuration" onClose={onClose} pending={pending}>
      <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <label className={labelClass}>Brand<input className={inputClass} autoFocus value={manufacturer} onChange={(event) => setManufacturer(event.target.value)} maxLength={200} required /></label>
        <label className={labelClass}>ชนิดอุปกรณ์<input className={inputClass} value={deviceType} onChange={(event) => setDeviceType(event.target.value)} maxLength={100} required /></label>
        <label className={`${labelClass} sm:col-span-2`}>รุ่น<input className={inputClass} value={modelName} onChange={(event) => setModelName(event.target.value)} maxLength={200} required /></label>
        <label className={labelClass}>Source Type ID<input className={inputClass} type="number" min="0" value={sourceTypeId} onChange={(event) => setSourceTypeId(event.target.value)} /></label>
        <label className="flex items-center gap-2 self-end text-sm font-bold text-slate-800"><input className="h-4 w-4 accent-[#174f68]" type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /> เปิดใช้งาน</label>
        {error && <p className="rounded-md bg-rose-50 px-3 py-2 text-sm font-bold text-rose-700 sm:col-span-2">{error}</p>}
        <div className="flex justify-end gap-2 sm:col-span-2"><button type="button" className={secondaryButtonClass} onClick={onClose} disabled={pending}>ยกเลิก</button><button className={primaryButtonClass} disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</button></div>
      </form>
    </Dialog>
  );
}

function AddressMetadataDialog({ model, item, onClose, onSaved }: { model: DeviceModelOption; item: RegisterMetadata | null; onClose: () => void; onSaved: (item: RegisterMetadata) => void }) {
  const [addressKey, setAddressKey] = useState(item?.addressKey ?? "");
  const [displayName, setDisplayName] = useState(item?.displayName ?? "");
  const [unit, setUnit] = useState(item?.unit ?? "");
  const [dataType, setDataType] = useState<RegisterMetadata["dataType"]>(item?.dataType ?? "number");
  const [scale, setScale] = useState(String(item?.scale ?? 1));
  const [offset, setOffset] = useState(String(item?.offset ?? 0));
  const [decimals, setDecimals] = useState(String(item?.decimals ?? 2));
  const [isEnabled, setIsEnabled] = useState(item?.isEnabled ?? true);
  const [notes, setNotes] = useState(item?.notes ?? "");
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
        body: JSON.stringify({ addressKey, displayName, unit, dataType, scale: Number(scale), offset: Number(offset), decimals: Number(decimals), isEnabled, notes }),
      });
      if (!response.ok) throw new Error(response.status === 404 ? "ไม่พบ Device Model" : "Address Metadata ไม่ถูกต้องหรือไม่สามารถบันทึกได้");
      onSaved((await response.json()) as RegisterMetadata);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog title={item ? `แก้ไข ${item.addressKey}` : "เพิ่ม Address Metadata"} eyebrow={`${model.manufacturer} / ${model.model}`} onClose={onClose} pending={pending}>
      <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <label className={labelClass}>Address / Key<input className={inputClass} autoFocus value={addressKey} onChange={(event) => setAddressKey(event.target.value)} maxLength={200} readOnly={Boolean(item)} required /></label>
        <label className={labelClass}>Display name<input className={inputClass} value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} placeholder="Active power" /></label>
        <label className={labelClass}>Unit<input className={inputClass} value={unit} onChange={(event) => setUnit(event.target.value)} maxLength={40} placeholder="kW" /></label>
        <label className={labelClass}>Data type<select className={inputClass} value={dataType} onChange={(event) => setDataType(event.target.value as RegisterMetadata["dataType"])}><option value="number">number</option><option value="boolean">boolean</option><option value="text">text</option><option value="enum">enum</option></select></label>
        <label className={labelClass}>Scale<input className={inputClass} type="number" step="any" value={scale} onChange={(event) => setScale(event.target.value)} required /></label>
        <label className={labelClass}>Offset<input className={inputClass} type="number" step="any" value={offset} onChange={(event) => setOffset(event.target.value)} required /></label>
        <label className={labelClass}>Decimals<input className={inputClass} type="number" min="0" max="9" value={decimals} onChange={(event) => setDecimals(event.target.value)} required /></label>
        <label className="flex items-center gap-2 self-end text-sm font-bold text-slate-800"><input className="h-4 w-4 accent-[#174f68]" type="checkbox" checked={isEnabled} onChange={(event) => setIsEnabled(event.target.checked)} /> เปิดใช้งาน</label>
        <label className={`${labelClass} sm:col-span-2`}>Notes<textarea className={`${inputClass} min-h-24 py-2`} value={notes} onChange={(event) => setNotes(event.target.value)} maxLength={500} /></label>
        {error && <p className="rounded-md bg-rose-50 px-3 py-2 text-sm font-bold text-rose-700 sm:col-span-2">{error}</p>}
        <div className="flex justify-end gap-2 sm:col-span-2"><button type="button" className={secondaryButtonClass} onClick={onClose} disabled={pending}>ยกเลิก</button><button className={primaryButtonClass} disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก Address"}</button></div>
      </form>
    </Dialog>
  );
}

function Dialog({ title, eyebrow, onClose, pending, children }: { title: string; eyebrow: string; onClose: () => void; pending: boolean; children: React.ReactNode }) {
  useEffect(() => {
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape" && !pending) onClose();
    }
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = "";
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [onClose, pending]);

  return (
    <div className="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-slate-950/55 p-4" role="presentation" onMouseDown={(event) => { if (!pending && event.target === event.currentTarget) onClose(); }}>
      <section className="max-h-[calc(100vh-2rem)] w-full max-w-2xl overflow-y-auto rounded-md bg-white shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="metadata-dialog-title">
        <header className="flex items-center justify-between gap-4 border-b border-slate-200 px-5 py-4">
          <div className="min-w-0"><p className="truncate text-xs font-extrabold uppercase text-slate-500">{eyebrow}</p><h2 id="metadata-dialog-title" className="truncate text-xl font-extrabold text-slate-900">{title}</h2></div>
          <button className={iconButtonClass} type="button" onClick={onClose} disabled={pending} title="ปิด" aria-label="ปิด"><X size={18} /></button>
        </header>
        <div className="p-5">{children}</div>
      </section>
    </div>
  );
}

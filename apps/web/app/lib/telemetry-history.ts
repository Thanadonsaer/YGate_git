import { api } from "./api.ts";
import type { Device, DeviceModelRegisterMetadata, LatestTelemetry, RegisterMetadata, TelemetryHistoryPage } from "./types.ts";

/** The server rejects anything above this (services/platform-api telemetry.go). */
const PAGE_LIMIT = 500;

/**
 * Pages to follow before giving up. 20 x 500 covers a month at 5-minute
 * polling; past that the caller shows a "truncated" badge rather than quietly
 * charting a slice of the range the user asked for.
 */
const MAX_PAGES = 20;

/**
 * Every reading in [from, to], oldest first.
 *
 * The endpoint returns newest-first pages with a keyset cursor, so this walks
 * backwards and reverses once at the end. Following the cursor matters: a
 * single 500-row page is only ~8 hours of 1-minute telemetry, so a 24-hour
 * request that stopped at page one would silently chart a third of the window
 * and every kWh total computed from it would be wrong.
 */
export async function fetchRange(
  plantId: string,
  deviceId: string,
  from: Date,
  to: Date,
  signal?: AbortSignal,
): Promise<{ readings: LatestTelemetry[]; truncated: boolean }> {
  const readings: LatestTelemetry[] = [];
  let cursor = "";
  for (let page = 0; page < MAX_PAGES; page++) {
    const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), limit: String(PAGE_LIMIT) });
    if (cursor) query.set("cursor", cursor);
    const response = await api(
      `/api/v1/plants/${encodeURIComponent(plantId)}/devices/${encodeURIComponent(deviceId)}/telemetry/history?${query}`,
      { signal },
    );
    if (!response.ok) {
      throw new Error(response.status === 404 ? "ไม่พบ Plant หรือ Device" : "ไม่สามารถโหลดข้อมูล telemetry ได้");
    }
    const body = (await response.json()) as TelemetryHistoryPage;
    readings.push(...body.data);
    if (!body.nextCursor) return { readings: readings.reverse(), truncated: false };
    cursor = body.nextCursor;
  }
  return { readings: readings.reverse(), truncated: true };
}

/**
 * Device history walks allowed in flight at once.
 *
 * Each device is its own sequential walk of up to MAX_PAGES requests, so firing
 * every selected device at once both exhausts the browser's six-connection
 * budget for the origin -- starving the register-metadata requests the same
 * page fires alongside these -- and hands the API that many concurrent
 * full-range scans. An eight-device selection came back as eight 502s and a
 * chart that never loaded, which is what this bounds.
 */
const CONCURRENT_DEVICES = 3;

/** fetchRange for several devices, results in device order, bounded fan-out. */
export async function fetchRanges(
  plantId: string,
  deviceIds: string[],
  from: Date,
  to: Date,
  signal?: AbortSignal,
) {
  const pages: Array<{ readings: LatestTelemetry[]; truncated: boolean }> = [];
  // ponytail: fixed batches, not a rolling pool -- the slowest device in a
  // batch holds up the next one. Swap for a worker pool if that ever shows.
  for (let index = 0; index < deviceIds.length; index += CONCURRENT_DEVICES) {
    const batch = deviceIds.slice(index, index + CONCURRENT_DEVICES);
    pages.push(...(await Promise.all(batch.map((id) => fetchRange(plantId, id, from, to, signal)))));
  }
  return pages;
}

/** What the point picker and the chart need to know about one register. */
export type PointMeta = {
  key: string;
  displayName: string;
  unit: string;
  decimals: number;
  /** Modbus address shown as the option's tag, e.g. "FC3:40071". */
  tag: string;
  isEnabled: boolean;
};

/**
 * Display names and units live on the plant-level register metadata; the Modbus
 * address lives on the device *model*. Merge both so one lookup answers "what
 * do I call this, what unit is it in, and which register is it".
 */
export async function loadRegisterCatalog(plantId: string, device: Device, signal?: AbortSignal) {
  return buildCatalog(plantId, device, signal, loadModelTags);
}

/**
 * loadRegisterCatalog for every device at once, keyed by device id.
 *
 * Devices in a plant overwhelmingly share a handful of models, so the
 * model-level lookup is fetched once per model rather than once per device --
 * a 40-inverter plant is 41 requests instead of 80. A device whose own
 * metadata fails is left out rather than failing the whole batch: the pickers
 * that use this fall back to the raw address key, which is what they showed
 * before this existed.
 */
export async function loadRegisterCatalogs(plantId: string, devices: Device[], signal?: AbortSignal) {
  const modelCache = new Map<string, Promise<Map<string, string>>>();
  const cachedModelTags = (deviceModelId: string) => {
    let tags = modelCache.get(deviceModelId);
    if (!tags) {
      tags = loadModelTags(deviceModelId, signal);
      modelCache.set(deviceModelId, tags);
    }
    return tags;
  };
  const entries = await Promise.all(devices.map(async (device) => {
    const catalog = await buildCatalog(plantId, device, signal, cachedModelTags).catch(() => ({}));
    return [device.id, catalog] as const;
  }));
  // Each device swallows its own failure above, and `api` turns an aborted
  // fetch into a 503 rather than a rejection -- so without this a cancelled
  // batch resolves cleanly with empty catalogs and the caller's `.then` writes
  // those over the catalogs the run that replaced it has already loaded.
  signal?.throwIfAborted();
  return Object.fromEntries(entries) as Record<string, Record<string, PointMeta>>;
}

/**
 * One option per register for a point picker, labelled by display name and
 * tagged with the Modbus address.
 *
 * Values are the bare address ("40072", not "reg40072"): alarm rules and SCADA
 * bindings are both matched against telemetry's dataItemMap, which
 * telemetry.mapped_data_items() emits keyed by the bare address. The catalog
 * indexes both spellings onto the same object, so dedupe is by identity.
 */
export function pointOptions(catalog: Record<string, PointMeta>) {
  const seen = new Set<PointMeta>();
  const options: Array<{ label: string; value: string; unit: string; tag: string }> = [];
  for (const meta of Object.values(catalog)) {
    if (seen.has(meta) || !meta.isEnabled) continue;
    seen.add(meta);
    options.push({ label: meta.displayName, value: bareAddressKey(meta.key) || meta.key, unit: meta.unit, tag: meta.tag });
  }
  return options.sort((a, b) => a.label.localeCompare(b.label));
}

/** The address tag lives on the device *model*, shared across its devices. */
async function loadModelTags(deviceModelId: string, signal?: AbortSignal): Promise<Map<string, string>> {
  // The tag is decoration: if the model lookup fails the picker still shows
  // names and units, so degrade instead of blanking the whole page.
  const modelLevel = await fetchJSON<DeviceModelRegisterMetadata[]>(
    `/api/v1/device-models/${encodeURIComponent(deviceModelId)}/register-metadata`,
    signal,
    "",
  ).catch(() => [] as DeviceModelRegisterMetadata[]);
  return new Map(modelLevel.map((entry) => [entry.addressKey, registerTag(entry)]));
}

async function buildCatalog(
  plantId: string,
  device: Device,
  signal: AbortSignal | undefined,
  modelTags: (deviceModelId: string, signal?: AbortSignal) => Promise<Map<string, string>>,
) {
  const plantLevel = await fetchJSON<RegisterMetadata[]>(
    `/api/v1/plants/${encodeURIComponent(plantId)}/device-register-metadata/${encodeURIComponent(device.id)}`,
    signal,
    "ไม่สามารถโหลด Metadata ของ Parameter ได้",
  );
  const tagByKey = device.deviceModelId ? await modelTags(device.deviceModelId, signal) : new Map<string, string>();
  const catalog: Record<string, PointMeta> = {};
  for (const entry of plantLevel) {
    const meta: PointMeta = {
      key: entry.addressKey,
      displayName: entry.displayName || entry.addressKey,
      unit: entry.unit,
      decimals: entry.decimals,
      tag: tagByKey.get(entry.addressKey) || entry.addressKey,
      isEnabled: entry.isEnabled,
    };
    catalog[entry.addressKey] = meta;
    // Metadata is keyed "reg40072" (what auto-onboard and import-config write)
    // but telemetry.mapped_data_items() emits the bare address "40072", so a
    // lookup by the telemetry key misses every row and the picker falls back
    // to showing the address instead of the display name. Index both spellings
    // -- the same both-forms match that SQL function already does on its side.
    const bare = bareAddressKey(entry.addressKey);
    if (bare && !catalog[bare]) catalog[bare] = meta;
  }
  return catalog;
}

/**
 * "reg40072" -> "40072". Empty for anything else, so a curated named key like
 * "regulation_mode" never aliases itself to a truncated "ulation_mode".
 */
export function bareAddressKey(addressKey: string): string {
  const match = /^reg(\d+(?:_fc\d+)?)$/.exec(addressKey);
  return match ? match[1] : "";
}

/** Fills in a placeholder for a key that has telemetry but no metadata row. */
export function pointMeta(catalog: Record<string, PointMeta>, key: string): PointMeta {
  return catalog[key] ?? { key, displayName: key, unit: "", decimals: 2, tag: key, isEnabled: true };
}

export function formatValue(value: number | undefined, meta: PointMeta) {
  if (value === undefined || !Number.isFinite(value)) return "-";
  return value.toLocaleString(undefined, { minimumFractionDigits: meta.decimals, maximumFractionDigits: meta.decimals });
}

function registerTag(entry: DeviceModelRegisterMetadata) {
  if (entry.modbusRegister == null) return "";
  return entry.modbusFunctionCode == null
    ? String(entry.modbusRegister)
    : `FC${entry.modbusFunctionCode}:${entry.modbusRegister}`;
}

async function fetchJSON<T>(path: string, signal: AbortSignal | undefined, message: string): Promise<T> {
  const response = await api(path, { signal });
  if (!response.ok) throw new Error(message || `request failed: ${response.status}`);
  return (await response.json()) as T;
}

import { api } from "./api";
import type { Device, DeviceModelRegisterMetadata, LatestTelemetry, RegisterMetadata, TelemetryHistoryPage } from "./types";

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
  const plantLevel = await fetchJSON<RegisterMetadata[]>(
    `/api/v1/plants/${encodeURIComponent(plantId)}/device-register-metadata/${encodeURIComponent(device.id)}`,
    signal,
    "ไม่สามารถโหลด Metadata ของ Parameter ได้",
  );
  // The address tag is decoration: if the model lookup fails the picker still
  // shows names and units, so degrade instead of blanking the whole page.
  const modelLevel = device.deviceModelId
    ? await fetchJSON<DeviceModelRegisterMetadata[]>(
      `/api/v1/device-models/${encodeURIComponent(device.deviceModelId)}/register-metadata`,
      signal,
      "",
    ).catch(() => [])
    : [];

  const tagByKey = new Map(modelLevel.map((entry) => [entry.addressKey, registerTag(entry)]));
  const catalog: Record<string, PointMeta> = {};
  for (const entry of plantLevel) {
    catalog[entry.addressKey] = {
      key: entry.addressKey,
      displayName: entry.displayName || entry.addressKey,
      unit: entry.unit,
      decimals: entry.decimals,
      tag: tagByKey.get(entry.addressKey) || entry.addressKey,
      isEnabled: entry.isEnabled,
    };
  }
  return catalog;
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

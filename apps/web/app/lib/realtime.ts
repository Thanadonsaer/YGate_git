"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { apiJson, errorMessage, gatewayURL } from "./api";
import { loadRegisterCatalogs, type PointMeta } from "./telemetry-history";
import { applyTelemetrySnapshot, latestByDeviceId } from "./telemetry-math";
import type { ConnectionState, Device, LatestTelemetry, LiveMessage } from "./types";

// Shared by platform-shell (no plantId, connection-state only), the Plants
// device dialog, the SCADA viewer and the Alarms page — each previously
// reimplemented this same connect/reconnect-with-backoff loop against
// GET /api/v1/realtime.
export function useRealtimeSocket(plantId: string | undefined, onMessage: (message: LiveMessage) => void, enabled = true): ConnectionState {
  const [state, setState] = useState<ConnectionState>("connecting");
  const handlerRef = useRef(onMessage);
  handlerRef.current = onMessage;

  useEffect(() => {
    if (!enabled) return;
    let stopped = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let socket: WebSocket | undefined;
    function connect() {
      if (stopped) return;
      setState("connecting");
      const query = plantId ? `?plantId=${encodeURIComponent(plantId)}` : "";
      socket = new WebSocket(`${gatewayURL.replace(/^http/, "ws")}/api/v1/realtime${query}`);
      socket.onmessage = (event) => {
        const message = JSON.parse(event.data as string) as LiveMessage;
        setState("connected");
        handlerRef.current(message);
      };
      socket.onclose = () => {
        if (stopped) return;
        setState("offline");
        retryTimer = setTimeout(connect, 3000);
      };
      socket.onerror = () => socket?.close();
    }
    connect();
    return () => {
      stopped = true;
      if (retryTimer) clearTimeout(retryTimer);
      socket?.close();
    };
  }, [plantId, enabled]);

  return state;
}

/**
 * A plant's devices and their latest telemetry, kept live by the realtime
 * socket -- what the Plants device view, the SCADA builder/viewer and the
 * dashboard chart picker all need. A failed load is surfaced as `error` (the
 * previous data is cleared only when the plant changes, not on a failed
 * reload). `catalogs` (register display names) is opt-in because it costs a
 * request per device and is best effort: a device whose metadata fails simply
 * has no entry, and pickers fall back to the raw address key.
 */
export function usePlantTelemetry(plantId: string | undefined, options: { catalogs?: boolean } = {}) {
  const withCatalogs = options.catalogs ?? false;
  const [devices, setDevices] = useState<Device[]>([]);
  const [latestByDevice, setLatestByDevice] = useState<Record<string, LatestTelemetry>>({});
  const [catalogs, setCatalogs] = useState<Record<string, Record<string, PointMeta>>>({});
  const [loading, setLoading] = useState(Boolean(plantId));
  const [error, setError] = useState("");
  const [reloadTick, setReloadTick] = useState(0);
  const reload = useCallback(() => setReloadTick((tick) => tick + 1), []);

  useEffect(() => {
    setDevices([]);
    setLatestByDevice({});
    setCatalogs({});
    setError("");
  }, [plantId]);

  useEffect(() => {
    if (!plantId) {
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    const base = `/api/v1/plants/${encodeURIComponent(plantId)}`;
    const request = { signal: controller.signal, messages: { 404: "ไม่พบโรงไฟฟ้าหรือบัญชีนี้ไม่มีสิทธิ์เข้าถึง Device", default: "ไม่สามารถโหลดข้อมูล Device ได้" } };
    setLoading(true);
    setError("");
    Promise.all([apiJson<Device[]>(`${base}/devices`, request), apiJson<LatestTelemetry[]>(`${base}/telemetry/latest`, request)])
      .then(([nextDevices, readings]) => {
        if (controller.signal.aborted) return;
        setDevices(nextDevices);
        setLatestByDevice(latestByDeviceId(readings));
        if (withCatalogs) void loadRegisterCatalogs(plantId, nextDevices, controller.signal).then(setCatalogs).catch(() => undefined);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [plantId, reloadTick, withCatalogs]);

  const liveState = useRealtimeSocket(plantId, (message) => {
    if (message.type === "telemetry.snapshot") setLatestByDevice((current) => applyTelemetrySnapshot(current, message, plantId));
  }, Boolean(plantId));

  return { devices, latestByDevice, catalogs, loading, error, reload, liveState };
}

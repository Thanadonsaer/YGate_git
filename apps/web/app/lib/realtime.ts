"use client";

import { useEffect, useRef, useState } from "react";
import { gatewayURL } from "./api";
import type { ConnectionState, LiveMessage } from "./types";

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

export type MiddlewareProgressState = {
  uploading: boolean;
  staging: boolean;
  applying: boolean;
  rollingBack: boolean;
  restarting: boolean;
};

type DownloadProgressItem = {
  status: string;
  startedAt?: string;
  downloadedBytes?: number;
  totalBytes?: number;
};

export function estimateDownloadRemainingMs(items: DownloadProgressItem[], now = Date.now()): number | null {
  const estimates = items.flatMap((item) => {
    const downloaded = item.downloadedBytes || 0;
    const total = item.totalBytes || 0;
    const startedAt = item.startedAt ? Date.parse(item.startedAt) : NaN;
    const elapsed = now - startedAt;
    if (item.status !== "running" || downloaded <= 0 || total <= downloaded || !Number.isFinite(startedAt) || elapsed <= 0) return [];
    return [Math.round(((total - downloaded) / (downloaded / elapsed)))];
  });
  return estimates.length === 0 ? null : Math.max(...estimates);
}

export function estimateRemainingMs(items: Array<{ status: string; durationMs?: number }>): number | null {
  const remaining = items.filter((item) => item.status !== "succeeded" && item.status !== "failed").length;
  if (remaining === 0) return 0;
  const completed = items.filter((item) => (item.status === "succeeded" || item.status === "failed") && (item.durationMs || 0) > 0);
  if (completed.length === 0) return null;
  const average = completed.reduce((total, item) => total + (item.durationMs || 0), 0) / completed.length;
  return Math.round(average * remaining);
}

export function middlewareProgressLabel(state: MiddlewareProgressState): string | null {
  if (state.uploading) return "กำลัง Upload patch...";
  if (state.staging) return "กำลังส่งไปยัง Middleware...";
  if (state.applying) return "กำลัง Apply patch...";
  if (state.rollingBack) return "กำลัง Rollback...";
  if (state.restarting) return "กำลัง Restart service...";
  return null;
}

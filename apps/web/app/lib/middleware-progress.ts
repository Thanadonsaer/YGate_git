export type MiddlewareProgressState = {
  uploading: boolean;
  staging: boolean;
  applying: boolean;
  rollingBack: boolean;
  restarting: boolean;
};

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

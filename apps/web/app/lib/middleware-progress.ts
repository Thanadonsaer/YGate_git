export type MiddlewareProgressState = {
  uploading: boolean;
  staging: boolean;
  applying: boolean;
  rollingBack: boolean;
  restarting: boolean;
};

export function middlewareProgressLabel(state: MiddlewareProgressState): string | null {
  if (state.uploading) return "กำลัง Upload patch...";
  if (state.staging) return "กำลัง Stage patch...";
  if (state.applying) return "กำลัง Apply patch...";
  if (state.rollingBack) return "กำลัง Rollback...";
  if (state.restarting) return "กำลัง Restart service...";
  return null;
}

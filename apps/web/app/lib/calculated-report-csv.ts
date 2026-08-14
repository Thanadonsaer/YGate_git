export type CalculatedReportRow = {
  plant: string;
  device: string;
  observedAt: string;
  values: Record<string, number>;
  displayValues?: Record<string, string>;
  kWh?: number | null;
};

function cell(value: unknown): string {
  const text = value == null ? "" : String(value);
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

export function calculatedReportCSV(rows: CalculatedReportRow[]): string {
  const keys = [...new Set(rows.flatMap((row) => Object.keys(row.values)))].sort();
  const displayKeys = new Set(rows.flatMap((row) => Object.keys(row.displayValues ?? {})));
  const valueHeaders = keys.flatMap((key) => displayKeys.has(key) ? [`${key}_numeric`, `${key}_display`] : [key]);
  const header = ["Plant", "Device", "Observed at", ...valueHeaders, "kWh"];
  return [header, ...rows.map((row) => [row.plant, row.device, row.observedAt, ...keys.flatMap((key) => displayKeys.has(key) ? [row.values[key], row.displayValues?.[key] ?? ""] : [row.values[key]]), row.kWh])]
    .map((row) => row.map(cell).join(","))
    .join("\r\n");
}

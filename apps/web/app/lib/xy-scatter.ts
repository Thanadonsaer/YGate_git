export type ScatterSample = { t: number; v: number };
export type ScatterPoint = { x: number; y: number; t: number };

export function buildScatterPoints(xSeries: readonly ScatterSample[], ySeries: readonly ScatterSample[]): ScatterPoint[] {
  const yByTime = new Map(ySeries.map((sample) => [sample.t, sample.v]));
  return xSeries
    .map((sample) => ({ x: sample.v, y: yByTime.get(sample.t), t: sample.t }))
    .filter((point): point is ScatterPoint => point.y !== undefined && Number.isFinite(point.x) && Number.isFinite(point.y))
    .sort((a, b) => a.t - b.t);
}

export function findSignalKey(keys: readonly string[], pattern: RegExp): string | undefined {
  return keys.find((key) => pattern.test(key));
}

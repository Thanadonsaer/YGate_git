export function calculateCapacityFactor(activePowerKw: number, installedAcKw: number): number | null {
  if (!Number.isFinite(activePowerKw) || !Number.isFinite(installedAcKw) || installedAcKw <= 0) return null;
  return (activePowerKw / installedAcKw) * 100;
}

export function calculateTargetAchievement(actual: number, target: number): number | null {
  if (!Number.isFinite(actual) || !Number.isFinite(target) || target <= 0) return null;
  return (actual / target) * 100;
}

export function rankPortfolio<T extends { id: string; value: number }>(
  items: readonly T[],
  value: (item: T) => number,
): Array<{ id: string; rank: number; value: number }> {
  return items
    .map((item, index) => ({ id: item.id, value: value(item), index }))
    .filter((item) => Number.isFinite(item.value))
    .sort((a, b) => b.value - a.value || a.index - b.index)
    .map((item, index) => ({ id: item.id, rank: index + 1, value: item.value }));
}

export function formatTokens(value: number, includeExact = true): string {
  const n = Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : 0;
  const exact = n.toLocaleString("en-US");
  const units: [number, string][] = [
    [1_000_000_000, "B"],
    [1_000_000, "M"],
    [1_000, "K"],
  ];
  for (const [size, suffix] of units) {
    if (n >= size) {
      const compact = `${(n / size).toFixed(n >= size * 10 ? 0 : 1)}${suffix}`;
      return includeExact ? `${compact} (${exact})` : compact;
    }
  }
  return exact;
}

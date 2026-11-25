/**
 * Deep-link parameters for record pages ("/hr?view=people&employee=<id>").
 * Apps read them once on mount to pick the starting view and open a record, so
 * "Open in ..." links from other apps land on the right record.
 */
export function deepParam(name: string, search: string = typeof window === "undefined" ? "" : window.location.search): string {
  return new URLSearchParams(search).get(name) ?? "";
}

/** deepParam restricted to a known set of values (anything else gives the fallback). */
export function deepChoice<T extends string>(name: string, allowed: readonly T[], fallback: T, search?: string): T {
  const v = deepParam(name, search);
  return (allowed as readonly string[]).includes(v) ? (v as T) : fallback;
}

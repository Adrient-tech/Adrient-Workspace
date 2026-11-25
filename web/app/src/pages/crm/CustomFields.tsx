import { PersonSelect } from "../../components/PeoplePicker";
import { Chip, FormControl, FormLabel, Input, Option, Select, Stack } from "@mui/joy";
import type { FieldDef, FieldEntity, UserLite } from "./types";

/** Which definitions apply to a record kind ("person" = lead + contact). */
export function defsFor(fields: FieldDef[], kind: "person" | "company" | "deal"): FieldDef[] {
  const want: FieldEntity[] = kind === "person" ? ["lead", "contact"] : [kind];
  return fields.filter((f) => want.includes(f.entity)).sort((a, b) => a.position - b.position || a.label.localeCompare(b.label));
}

export type CustomValues = Record<string, unknown>;

/** Human-readable value for lists, cards and filters. */
export function showValue(def: FieldDef, v: unknown, users: UserLite[] = []): string {
  if (v === undefined || v === null || v === "") return "";
  if (Array.isArray(v)) return v.join(", ");
  if (def.type === "user") return users.find((u) => u.id === v)?.name ?? String(v);
  return String(v);
}

/** True when the record's custom values satisfy "field = value" (contains, case-insensitive). */
export function matchesCustom(def: FieldDef | undefined, values: CustomValues | undefined, needle: string, users: UserLite[]): boolean {
  if (!def || !needle.trim()) return true;
  return showValue(def, values?.[def.key], users).toLowerCase().includes(needle.trim().toLowerCase());
}

/** Inputs for every custom field of a record kind. Returns null when there are none. */
export function CustomFieldInputs({ defs, value, onChange, users }: { defs: FieldDef[]; value: CustomValues; onChange: (v: CustomValues) => void; users: UserLite[] }) {
  if (defs.length === 0) return null;
  const set = (k: string, v: unknown) => onChange({ ...value, [k]: v });
  return (
    <Stack gap={1} data-testid="crm-custom-fields">
      {defs.map((d) => {
        const v = value[d.key];
        const label = d.label + (d.required ? " *" : "");
        return (
          <FormControl key={d.id} required={d.required}>
            <FormLabel>{label}</FormLabel>
            {d.type === "select" && (
              <Select value={(v as string) ?? ""} onChange={(_, x) => set(d.key, x ?? "")} slotProps={{ button: { "data-testid": `crm-cf-${d.key}` } as never }} aria-label={d.label}>
                <Option value="">-</Option>
                {d.options.map((o) => (
                  <Option key={o} value={o}>
                    {o}
                  </Option>
                ))}
              </Select>
            )}
            {d.type === "multiselect" && (
              <Select
                multiple
                value={Array.isArray(v) ? (v as string[]) : []}
                onChange={(_, x) => set(d.key, x)}
                aria-label={d.label}
                renderValue={(sel) => (
                  <Stack direction="row" gap={0.5} flexWrap="wrap">
                    {sel.map((o) => (
                      <Chip key={o.value} size="sm">
                        {o.label}
                      </Chip>
                    ))}
                  </Stack>
                )}
                slotProps={{ button: { "data-testid": `crm-cf-${d.key}` } as never }}
              >
                {d.options.map((o) => (
                  <Option key={o} value={o}>
                    {o}
                  </Option>
                ))}
              </Select>
            )}
            {d.type === "user" && (
              <PersonSelect value={(v as string) ?? ""} known={users} onChange={(id) => set(d.key, id)} ariaLabel={d.label} testId={`crm-cf-${d.key}`} />
            )}
            {(d.type === "text" || d.type === "url" || d.type === "number" || d.type === "date") && (
              <Input
                type={d.type === "number" ? "number" : d.type === "date" ? "date" : d.type === "url" ? "url" : "text"}
                value={(v as string | number | undefined) ?? ""}
                onChange={(e) => set(d.key, e.target.value)}
                slotProps={{ input: { "data-testid": `crm-cf-${d.key}`, "aria-label": d.label } }}
              />
            )}
          </FormControl>
        );
      })}
    </Stack>
  );
}

/** Small "field contains value" filter for lists. */
export function CustomFilter({ defs, field, needle, onChange }: { defs: FieldDef[]; field: string; needle: string; onChange: (field: string, needle: string) => void }) {
  if (defs.length === 0) return null;
  return (
    <Stack direction="row" gap={0.5} alignItems="center" data-testid="crm-custom-filter">
      <Select size="sm" value={field} onChange={(_, v) => onChange(v ?? "", needle)} sx={{ width: 150 }} aria-label="Custom field filter" placeholder="Custom field">
        <Option value="">Any custom field</Option>
        {defs.map((d) => (
          <Option key={d.id} value={d.key}>
            {d.label}
          </Option>
        ))}
      </Select>
      {field && <Input size="sm" placeholder="contains" value={needle} onChange={(e) => onChange(field, e.target.value)} sx={{ width: 120 }} slotProps={{ input: { "aria-label": "Custom field value" } }} />}
    </Stack>
  );
}

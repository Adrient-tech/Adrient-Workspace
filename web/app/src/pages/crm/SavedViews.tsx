import { useCallback, useEffect, useState } from "react";
import { Button, Checkbox, Chip, FormControl, FormLabel, Input, Stack } from "@mui/joy";
import * as api2 from "./api2";
import type { SavedViewRow } from "./types";
import { Dialog, ErrorAlert, useErr } from "./ui";

/**
 * Saved views stored on the server per user. A view can be shared with the whole
 * org (everyone sees it, only its owner edits or deletes it).
 */
export function SavedViews({
  entity,
  getConfig,
  onApply,
  onClear,
}: {
  entity: "lead" | "company" | "deal" | "activity";
  getConfig: () => Record<string, unknown>;
  onApply: (c: Record<string, unknown>) => void;
  onClear: () => void;
}) {
  const [views, setViews] = useState<SavedViewRow[]>([]);
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  const [active, setActive] = useState("");
  const { err, run } = useErr();

  const load = useCallback(() => {
    api2.getViews(entity).then(setViews);
  }, [entity]);
  useEffect(load, [load]);

  const save = async () => {
    const r = await run(() => api2.save<SavedViewRow>("views", "", { entity, name: name.trim(), config: getConfig(), shared }));
    if (r) {
      setNaming(false);
      setName("");
      setShared(false);
      setActive(r.id);
      load();
    }
  };

  return (
    <>
      <Stack direction="row" gap={0.5} flexWrap="wrap" alignItems="center" data-testid="crm-views">
        <Button size="sm" variant="plain" onClick={() => setNaming(true)} data-testid="crm-save-view">
          Save view
        </Button>
        {views.map((v) => (
          <Chip
            key={v.id}
            size="sm"
            variant={active === v.id ? "solid" : "soft"}
            data-testid={`crm-view-${v.name}`}
            onClick={() => {
              setActive(v.id);
              onApply(v.config);
            }}
            endDecorator={
              v.mine ? (
                <span
                  role="button"
                  aria-label={`Delete view ${v.name}`}
                  onClick={async (e) => {
                    e.stopPropagation();
                    if (await run(() => api2.remove("views", v.id))) {
                      if (active === v.id) setActive("");
                      load();
                    }
                  }}
                >
                  ×
                </span>
              ) : undefined
            }
          >
            {v.name}
            {v.shared && !v.mine ? " (shared)" : v.shared ? " (shared by you)" : ""}
          </Chip>
        ))}
        {views.length > 0 && (
          <Chip
            size="sm"
            variant="plain"
            onClick={() => {
              setActive("");
              onClear();
            }}
          >
            Clear filters
          </Chip>
        )}
      </Stack>
      <ErrorAlert err={err} />
      {naming && (
        <Dialog title="Save view" onClose={() => setNaming(false)} width={420}>
          <Stack gap={1.5}>
            <FormControl>
              <FormLabel>Name</FormLabel>
              <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "crm-view-name" } }} />
            </FormControl>
            <Checkbox label="Share with everyone in the org" checked={shared} onChange={(e) => setShared(e.target.checked)} data-testid="crm-view-shared" />
            <Button disabled={!name.trim()} onClick={save} data-testid="crm-view-save">
              Save
            </Button>
          </Stack>
        </Dialog>
      )}
    </>
  );
}

import { useEffect, useMemo, useRef, useState } from "react";
import { Box, Chip, Input, List, ListItem, ListItemButton, ListItemContent, Modal, ModalDialog, Typography } from "@mui/joy";
import * as api2 from "./api2";

export interface Command {
  id: string;
  label: string;
  run: () => void;
}

const KIND: Record<string, string> = { lead: "Lead", company: "Company", deal: "Deal", activity: "Activity" };

/** Ctrl-K / Cmd-K search across leads, companies, deals and activities, plus quick navigation. */
export function CommandPalette({ commands, onOpenHit, onClose }: { commands: Command[]; onOpenHit: (h: api2.SearchHit) => void; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<api2.SearchHit[]>([]);
  const [sel, setSel] = useState(0);
  const seq = useRef(0);

  useEffect(() => {
    const term = q.trim();
    if (!term) {
      setHits([]);
      return;
    }
    const mine = ++seq.current;
    const t = setTimeout(() => {
      api2.search(term).then((h) => mine === seq.current && setHits(h)).catch(() => mine === seq.current && setHits([]));
    }, 120);
    return () => clearTimeout(t);
  }, [q]);

  const navs = useMemo(() => commands.filter((c) => !q.trim() || c.label.toLowerCase().includes(q.trim().toLowerCase())).slice(0, 6), [commands, q]);
  const rows: { key: string; label: string; sub?: string; tag: string; run: () => void }[] = [
    ...hits.map((h) => ({ key: `${h.kind}:${h.id}`, label: h.title || "(untitled)", sub: h.sub, tag: KIND[h.kind] ?? h.kind, run: () => onOpenHit(h) })),
    ...navs.map((c) => ({ key: `cmd:${c.id}`, label: c.label, tag: "Go to", run: c.run })),
  ];
  useEffect(() => setSel(0), [q, hits.length]);

  return (
    <Modal open onClose={onClose}>
      <ModalDialog layout="center" sx={{ width: { xs: "95vw", sm: 560 }, p: 1.5, top: "20%", transform: "translateX(-50%)" }} role="dialog" aria-label="Command search" data-testid="crm-palette">
        <Input
          autoFocus
          placeholder="Search leads, companies, deals, activities, or jump to a page"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          slotProps={{ input: { "aria-label": "Search CRM", "data-testid": "crm-palette-input" } }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") { e.preventDefault(); setSel((s) => Math.min(rows.length - 1, s + 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setSel((s) => Math.max(0, s - 1)); }
            else if (e.key === "Enter" && rows[sel]) { e.preventDefault(); rows[sel].run(); onClose(); }
          }}
        />
        <List sx={{ maxHeight: 360, overflow: "auto", mt: 1 }} data-testid="crm-palette-results">
          {rows.length === 0 && <Typography level="body-sm" sx={{ p: 1 }}>{q.trim() ? "Nothing found." : "Start typing to search."}</Typography>}
          {rows.map((r, i) => (
            <ListItem key={r.key}>
              <ListItemButton selected={i === sel} onClick={() => { r.run(); onClose(); }} data-testid="crm-palette-row">
                <Chip size="sm" variant="soft" sx={{ mr: 1, minWidth: 64, justifyContent: "center" }}>{r.tag}</Chip>
                <ListItemContent>
                  <Typography level="body-md">{r.label}</Typography>
                  {r.sub && <Box component="span"><Typography level="body-xs">{r.sub}</Typography></Box>}
                </ListItemContent>
              </ListItemButton>
            </ListItem>
          ))}
        </List>
      </ModalDialog>
    </Modal>
  );
}

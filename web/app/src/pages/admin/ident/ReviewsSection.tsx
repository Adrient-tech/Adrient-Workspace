// Access reviews: campaigns for group membership, external guests and admin
// roles. Reviewers decide keep or remove; closing applies the removals.
import { LoadOlder } from "../../../components/LoadOlder";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { Notice, Page, api, when } from "./common";

interface Campaign {
  id: string;
  name: string;
  kind: string;
  due_at: string;
  on_no_response: string;
  status: string;
  total: number;
  pending: number;
  kept: number;
  removed: number;
}
interface Item {
  id: string;
  subject_email: string;
  subject_name: string;
  resource_type: string;
  resource_name: string;
  reviewer_name: string;
  decision: string;
  comment: string;
  apply_result: string;
}
const KIND: Record<string, string> = { group_membership: "Group membership", external_guests: "External guests", admin_roles: "Admin roles" };

export function ReviewsSection() {
  const [list, setList] = useState<Campaign[]>([]);
  const [open, setOpen] = useState<{ campaign: Campaign; items: Item[]; next_cursor?: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [kind, setKind] = useState("group_membership");
  const [days, setDays] = useState("14");
  const [noResp, setNoResp] = useState("keep");
  const [repeat, setRepeat] = useState("");

  const load = useCallback(async () => {
    try {
      setList((await api<{ campaigns: Campaign[] }>("/api/v1/admin/reviews")).campaigns);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function show(id: string) {
    setOpen(await api(`/api/v1/admin/reviews/${id}`));
  }
  async function create() {
    setError(null);
    setNotice(null);
    try {
      const c = await api<Campaign>("/api/v1/admin/reviews", {
        method: "POST",
        json: { name, kind, due_days: Number(days), on_no_response: noResp, recurrence_days: repeat ? Number(repeat) : null },
      });
      setNotice(`Campaign started with ${c.total} decisions. Reviewers were notified.`);
      setName("");
      await load();
      await show(c.id);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function close(id: string) {
    setError(null);
    try {
      const c = await api<Campaign>(`/api/v1/admin/reviews/${id}/close`, { method: "POST" });
      setNotice(`Closed. ${c.removed} access(es) removed, ${c.kept} kept.`);
      await load();
      await show(id);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function decide(it: Item, d: "keep" | "remove") {
    try {
      await api(`/api/v1/admin/reviews/${open?.campaign.id}/items/${it.id}`, { method: "POST", json: { decision: d } });
      setOpen((o) => (o ? { ...o, items: o.items.map((x) => (x.id === it.id ? { ...x, decision: d } : x)) } : o));
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Page testId="reviews" title="Access reviews" intro="Ask the right person to confirm that group members, external guests and admins still need their access. Removals are applied automatically when the campaign closes.">
      <Notice error={error} notice={notice} />
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 760, mb: 3, display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end" }}>
        <FormControl size="sm">
          <FormLabel>Name</FormLabel>
          <Input size="sm" value={name} onChange={(e) => setName(e.target.value)} placeholder="Q4 guest review" slotProps={{ input: { "data-testid": "rev-name" } as never }} />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>What to review</FormLabel>
          <Select size="sm" value={kind} onChange={(_, v) => v && setKind(v)} sx={{ width: 190 }} data-testid="rev-kind">
            {Object.entries(KIND).map(([k, l]) => (
              <Option key={k} value={k}>
                {l}
              </Option>
            ))}
          </Select>
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Due in (days)</FormLabel>
          <Input size="sm" sx={{ width: 90 }} value={days} onChange={(e) => setDays(e.target.value)} />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>No answer means</FormLabel>
          <Select size="sm" value={noResp} onChange={(_, v) => v && setNoResp(v)} sx={{ width: 120 }}>
            <Option value="keep">Keep</Option>
            <Option value="remove">Remove</Option>
          </Select>
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Repeat every (days)</FormLabel>
          <Input size="sm" sx={{ width: 90 }} placeholder="once" value={repeat} onChange={(e) => setRepeat(e.target.value)} />
        </FormControl>
        <Button size="sm" disabled={!name.trim()} onClick={create} data-testid="rev-create">
          Start review
        </Button>
      </Sheet>

      <Table size="sm" sx={{ maxWidth: 900, mb: 2 }} data-testid="rev-list">
        <thead>
          <tr>
            <th>Campaign</th>
            <th>Due</th>
            <th>Progress</th>
            <th>Status</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {list.map((c) => (
            <tr key={c.id}>
              <td>
                {c.name} <Chip size="sm">{KIND[c.kind]}</Chip>
              </td>
              <td>{when(c.due_at)}</td>
              <td>
                {c.total - c.pending}/{c.total} decided · {c.removed} to remove
              </td>
              <td>{c.status}</td>
              <td>
                <Button size="sm" variant="outlined" onClick={() => void show(c.id)} data-testid={`rev-open-${c.name}`}>
                  Open
                </Button>
              </td>
            </tr>
          ))}
          {list.length === 0 && (
            <tr>
              <td colSpan={5}>No campaigns yet.</td>
            </tr>
          )}
        </tbody>
      </Table>

      {open && (
        <Box data-testid="rev-detail">
          <Box sx={{ display: "flex", gap: 1.5, alignItems: "center", mb: 1 }}>
            <Typography level="title-md">{open.campaign.name}</Typography>
            <Chip size="sm">{open.campaign.status}</Chip>
            {open.campaign.status === "open" && (
              <Button size="sm" color="danger" variant="outlined" onClick={() => void close(open.campaign.id)} data-testid="rev-close">
                Close and apply
              </Button>
            )}
            <Button size="sm" variant="plain" component="a" href={`/api/v1/admin/reviews/${open.campaign.id}/export`}>
              Export CSV
            </Button>
          </Box>
          <Table size="sm" sx={{ maxWidth: 980 }}>
            <thead>
              <tr>
                <th>Person</th>
                <th>Access</th>
                <th>Reviewer</th>
                <th>Decision</th>
              </tr>
            </thead>
            <tbody>
              {open.items.map((it) => (
                <tr key={it.id} data-testid={`rev-item-${it.subject_email}-${it.resource_name}`}>
                  <td>
                    {it.subject_name} <Typography level="body-xs">{it.subject_email}</Typography>
                  </td>
                  <td>
                    {it.resource_type}: {it.resource_name}
                  </td>
                  <td>{it.reviewer_name}</td>
                  <td>
                    {open.campaign.status === "open" ? (
                      <Box sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                        <Button size="sm" variant={it.decision === "keep" ? "solid" : "outlined"} color="success" onClick={() => void decide(it, "keep")}>
                          Keep
                        </Button>
                        <Button size="sm" variant={it.decision === "remove" ? "solid" : "outlined"} color="danger" onClick={() => void decide(it, "remove")}>
                          Remove
                        </Button>
                      </Box>
                    ) : (
                      <>
                        {it.decision} {it.apply_result && <Typography level="body-xs">{it.apply_result}</Typography>}
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          <LoadOlder next={open.next_cursor ?? ""} shown={open.items.length} label="Load more items" testId="rev-more" loadMore={async (c) => {
            const r = await api<{ campaign: Campaign; items: Item[]; next_cursor?: string }>(`/api/v1/admin/reviews/${open.campaign.id}?page=${c}`);
            setOpen((o) => (o ? { ...o, items: [...o.items, ...r.items], next_cursor: r.next_cursor } : o));
          }} />
        </Box>
      )}
    </Page>
  );
}

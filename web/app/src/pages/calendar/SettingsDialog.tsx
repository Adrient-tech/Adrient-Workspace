import { useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Checkbox, CircularProgress, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Tab, TabList, TabPanel, Tabs, Typography } from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import * as api from "./extrasApi";
import { HOLIDAY_COUNTRIES, HOLIDAY_COUNTRY_KEY, detectCountry } from "./holidays";
import { DAYS, feedUrl, windowsError } from "./extrasLogic";

/** Calendar settings: working hours, import / export, subscribe link. */
export function SettingsDialog({ onClose, onImported }: { onClose: () => void; onImported: () => void }) {
  const [wh, setWh] = useState<api.WorkingHours | null>(null);
  const [err, setErr] = useState("");
  const [saved, setSaved] = useState(false);
  const [feed, setFeed] = useState("");
  const [msg, setMsg] = useState("");
  const file = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.getSettings().then((s) => setWh(s.working_hours.timezone === "UTC" ? { ...s.working_hours, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC" } : s.working_hours)).catch((e: Error) => setErr(e.message));
    api.getFeed().then((f) => setFeed(feedUrl(window.location.origin, f.path))).catch(() => undefined);
  }, []);

  async function save() {
    if (!wh) return;
    for (const ws of Object.values(wh.days)) {
      const m = windowsError(ws);
      if (m) return setErr(m);
    }
    setErr("");
    try {
      await api.saveSettings(wh);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  async function doImport(f: File) {
    setMsg("");
    setErr("");
    try {
      const r = await api.importIcs(await f.text());
      setMsg(`Imported ${r.imported} event${r.imported === 1 ? "" : "s"}. ${r.duplicates} already there, ${r.skipped} skipped.`);
      if (r.imported > 0) onImported();
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "92vh", overflow: "hidden", display: "flex", flexDirection: "column" }} data-testid="cal-settings">
        <ModalClose />
        <Typography level="title-lg">Calendar settings</Typography>
        <Tabs defaultValue={0} sx={{ bgcolor: "transparent", minHeight: 0, flex: 1, overflow: "hidden" }}>
          <TabList>
            <Tab>Working hours</Tab>
            <Tab>Import and export</Tab>
            <Tab data-testid="tab-holidays">Holidays</Tab>
          </TabList>
          <TabPanel value={0} sx={{ overflowY: "auto" }}>
            {!wh && <CircularProgress />}
            {wh && (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <Typography level="body-sm">
                  Used by Find a time, so guests are not offered hours you do not work.
                </Typography>
                <Input value={wh.timezone} onChange={(e) => setWh({ ...wh, timezone: e.target.value })} startDecorator="Time zone" sx={{ maxWidth: 340 }} slotProps={{ input: { "data-testid": "wh-tz" } }} />
                {DAYS.map(({ key, label }) => {
                  const ws = wh.days[key] ?? [];
                  const set = (n: typeof ws) => setWh({ ...wh, days: { ...wh.days, [key]: n } });
                  const e = windowsError(ws);
                  return (
                    <Box key={key} sx={{ display: "flex", gap: 1, alignItems: "flex-start", flexWrap: "wrap" }}>
                      <Checkbox label={label} sx={{ width: 120, pt: 0.75 }} checked={ws.length > 0} onChange={(ev) => set(ev.target.checked ? [{ start: "09:00", end: "17:00" }] : [])} />
                      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
                        {ws.map((w, i) => (
                          <Box key={i} sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                            <Input size="sm" type="time" value={w.start} sx={{ width: 120 }} onChange={(ev) => set(ws.map((x, j) => (j === i ? { ...x, start: ev.target.value } : x)))} />
                            <span>–</span>
                            <Input size="sm" type="time" value={w.end} sx={{ width: 120 }} onChange={(ev) => set(ws.map((x, j) => (j === i ? { ...x, end: ev.target.value } : x)))} />
                            <IconButton size="sm" variant="plain" aria-label={`Remove ${label} window`} onClick={() => set(ws.filter((_, j) => j !== i))}>
                              <DeleteIcon fontSize="small" />
                            </IconButton>
                          </Box>
                        ))}
                        {e && <Typography level="body-xs" color="danger">{e}</Typography>}
                      </Box>
                      {ws.length > 0 && (
                        <IconButton size="sm" variant="plain" aria-label={`Add ${label} window`} onClick={() => set([...ws, { start: "13:00", end: "17:00" }])}>
                          <AddIcon fontSize="small" />
                        </IconButton>
                      )}
                    </Box>
                  );
                })}
                <Box>
                  <Button onClick={save} data-testid="wh-save">{saved ? "Saved" : "Save working hours"}</Button>
                </Box>
              </Box>
            )}
          </TabPanel>
          <TabPanel value={1} sx={{ overflowY: "auto" }}>
            <Typography level="title-sm">Import</Typography>
            <Typography level="body-sm" sx={{ mb: 1 }}>
              Choose an .ics file exported from Google Calendar, Outlook or Apple Calendar. Importing the same file twice does not create duplicates.
            </Typography>
            <input ref={file} type="file" accept=".ics,text/calendar" hidden data-testid="ics-file" onChange={(e) => { const f = e.target.files?.[0]; if (f) void doImport(f); e.target.value = ""; }} />
            <Button variant="outlined" color="neutral" onClick={() => file.current?.click()}>Choose .ics file</Button>
            <Typography level="title-sm" sx={{ mt: 3 }}>Export</Typography>
            <Button component="a" href={api.EXPORT_URL} download variant="outlined" color="neutral" sx={{ mt: 0.5 }} data-testid="ics-export">Download my events (.ics)</Button>
            <Typography level="title-sm" sx={{ mt: 3 }}>Subscribe from another calendar</Typography>
            <Typography level="body-sm" sx={{ mb: 1 }}>
              Add this private link in Google Calendar, Outlook or Apple Calendar ("from URL"). Private events show as Busy. Anyone with the link can see it, so rotate it if it leaks.
            </Typography>
            <Input readOnly value={feed} slotProps={{ input: { "data-testid": "feed-url" } }} onFocus={(e) => e.target.select()} />
            <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
              <Button size="sm" variant="outlined" color="neutral" onClick={() => void navigator.clipboard?.writeText(feed)}>Copy link</Button>
              <Button size="sm" variant="plain" color="danger" onClick={async () => {
                if (!window.confirm("Create a new link? Calendars subscribed to the old link stop updating.")) return;
                try { const f = await api.getFeed(true); setFeed(feedUrl(window.location.origin, f.path)); } catch (e) { setErr((e as Error).message); }
              }}>Rotate link</Button>
            </Box>
          </TabPanel>
          <TabPanel value={2} sx={{ overflowY: "auto" }}>
            <HolidayPicker />
          </TabPanel>
        </Tabs>
        {msg && <Alert color="success" data-testid="ics-msg">{msg}</Alert>}
        {err && <Alert color="danger">{err}</Alert>}
      </ModalDialog>
    </Modal>
  );
}

function HolidayPicker() {
  const [v, setV] = useState(() => localStorage.getItem(HOLIDAY_COUNTRY_KEY) ?? "auto");
  const auto = detectCountry();
  const autoName = HOLIDAY_COUNTRIES.find((c) => c.id === auto)?.name;
  function pick(x: string) {
    setV(x);
    localStorage.setItem(HOLIDAY_COUNTRY_KEY, x);
    window.dispatchEvent(new Event("adrient-holiday-country"));
  }
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1, maxWidth: 360 }}>
      <Typography level="body-sm">
        National public holidays shown on the Holidays calendar. They are computed, never stored, and are not exported.
        Lunar holidays such as Diwali and Eid are not included.
      </Typography>
      <Select value={v} onChange={(_, x) => pick(x ?? "auto")} slotProps={{ button: { "data-testid": "holiday-country" } as never }}>
        <Option value="auto">{`Automatic${autoName ? ` (${autoName})` : " (none for your region)"}`}</Option>
        <Option value="">Off</Option>
        {HOLIDAY_COUNTRIES.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}
      </Select>
    </Box>
  );
}

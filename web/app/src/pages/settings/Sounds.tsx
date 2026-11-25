import { useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import { Box, Button, Container, Divider, Input, Link, Sheet, Slider, Switch, Typography } from "@mui/joy";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import StopIcon from "@mui/icons-material/Stop";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import {
  CATEGORIES,
  CATEGORY_LABELS,
  CATEGORY_OF,
  LOOPING,
  SET_LABELS,
  SOUND_IDS,
  SOUND_LABELS,
  SOUND_SETS,
  isDnd,
  saveSoundPrefs,
  sounds,
  useSoundPrefs,
  type SoundCategory,
  type SoundId,
  type SoundPrefs,
  type SoundSet,
} from "../../lib/sounds";

/** Settings > Sounds: volume, per-category switches, tone set, test buttons, quiet hours, desktop alerts. */
export default function SoundsPage({ user }: { user: User }) {
  const prefs = useSoundPrefs();
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [playing, setPlaying] = useState<SoundId | null>(null);
  const [perm, setPerm] = useState<string>(typeof Notification === "undefined" ? "unsupported" : Notification.permission);

  const update = (patch: Partial<SoundPrefs>) => {
    setError(null);
    saveSoundPrefs({ ...prefs, ...patch })
      .then(() => {
        setSaved(true);
        window.setTimeout(() => setSaved(false), 2000);
      })
      .catch((e: Error) => setError(`Could not save to your account: ${e.message}. Kept on this device.`));
  };

  const test = async (id: SoundId) => {
    sounds.markUnlocked();
    if (LOOPING.has(id)) {
      if (sounds.isLooping(id)) {
        sounds.stop(id);
        setPlaying(null);
        return;
      }
      sounds.stopAll();
      setPlaying(id);
      await sounds.loop(id, { force: true });
      window.setTimeout(() => {
        sounds.stop(id);
        setPlaying((p) => (p === id ? null : p));
      }, 6500);
      return;
    }
    await sounds.play(id, { force: true });
  };

  async function enableDesktop(on: boolean) {
    if (!on) return update({ desktop: false });
    if (typeof Notification === "undefined") return;
    const p = await Notification.requestPermission().catch(() => "denied" as NotificationPermission);
    setPerm(p);
    update({ desktop: p === "granted" });
  }

  const quiet = isDnd(prefs.dnd);

  return (
    <>
      <Header user={user} />
      <Container maxWidth="sm" sx={{ py: { xs: 3, sm: 5 } }} data-testid="sounds-page">
        <Link component={RouterLink} to="/settings" level="body-sm">
          ← Settings
        </Link>
        <Typography level="h2" sx={{ mt: 1, mb: 0.5 }}>
          Sounds
        </Typography>
        <Typography level="body-sm" sx={{ mb: 3, opacity: 0.7 }}>
          Ringtones, meeting chimes and notification tones. Saved to your account, so they follow you across devices.
        </Typography>
        {error && (
          <Sheet color="danger" variant="soft" sx={{ p: 2, mb: 2, borderRadius: "md" }}>
            <Typography color="danger" level="body-sm">
              {error}
            </Typography>
          </Sheet>
        )}

        <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 3, mb: 3 }}>
          <Typography level="title-md" sx={{ mb: 1.5 }}>
            Volume and tone set
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
            <Typography level="body-sm" sx={{ width: 64 }}>
              Volume
            </Typography>
            <Slider
              aria-label="Master volume"
              data-testid="sound-volume"
              min={0}
              max={100}
              value={Math.round(prefs.volume * 100)}
              onChange={(_, v) => sounds.setPrefs({ ...prefs, volume: (v as number) / 100 })}
              onChangeCommitted={(_, v) => update({ volume: (v as number) / 100 })}
            />
            <Typography level="body-sm" sx={{ width: 40, textAlign: "right" }}>
              {Math.round(prefs.volume * 100)}%
            </Typography>
          </Box>
          <Box sx={{ display: "flex", gap: 1, mt: 2, flexWrap: "wrap" }} role="radiogroup" aria-label="Tone set">
            {SOUND_SETS.map((s: SoundSet) => (
              <Button
                key={s}
                size="sm"
                role="radio"
                aria-checked={prefs.set === s}
                variant={prefs.set === s ? "solid" : "outlined"}
                color={prefs.set === s ? "primary" : "neutral"}
                data-testid={`sound-set-${s}`}
                onClick={() => {
                  update({ set: s });
                  window.setTimeout(() => void sounds.play("message", { force: true }), 50);
                }}
              >
                {SET_LABELS[s]}
              </Button>
            ))}
          </Box>
          <Typography level="body-xs" sx={{ mt: 1, opacity: 0.65 }}>
            Classic: warm bell. Soft: low and rounded. Bright: light and quick.
          </Typography>
        </Sheet>

        <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 3, mb: 3 }}>
          <Typography level="title-md" sx={{ mb: 1 }}>
            Quiet
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", py: 1 }}>
            <Box>
              <Typography level="title-sm">Do Not Disturb</Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                Silences every sound, including ringtones{quiet ? " (on now)" : ""}.
              </Typography>
            </Box>
            <Switch
              checked={prefs.dnd.on}
              data-testid="sound-dnd"
              onChange={(e) => update({ dnd: { ...prefs.dnd, on: e.target.checked } })}
            />
          </Box>
          <Divider />
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", py: 1, gap: 1, flexWrap: "wrap" }}>
            <Box>
              <Typography level="title-sm">Quiet hours</Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                Automatically silent between these times.
              </Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Input
                size="sm"
                type="time"
                value={prefs.dnd.from}
                disabled={!prefs.dnd.scheduled}
                onChange={(e) => e.target.value && update({ dnd: { ...prefs.dnd, from: e.target.value } })}
                slotProps={{ input: { "aria-label": "Quiet hours from" } }}
              />
              <Typography level="body-sm">to</Typography>
              <Input
                size="sm"
                type="time"
                value={prefs.dnd.to}
                disabled={!prefs.dnd.scheduled}
                onChange={(e) => e.target.value && update({ dnd: { ...prefs.dnd, to: e.target.value } })}
                slotProps={{ input: { "aria-label": "Quiet hours to" } }}
              />
              <Switch
                checked={prefs.dnd.scheduled}
                data-testid="sound-quiet-hours"
                onChange={(e) => update({ dnd: { ...prefs.dnd, scheduled: e.target.checked } })}
              />
            </Box>
          </Box>
          <Divider />
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", py: 1 }}>
            <Box>
              <Typography level="title-sm">Follow my system's reduced-motion setting</Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                When your OS asks for less motion, notification tones stay quiet. Calls still ring.
              </Typography>
            </Box>
            <Switch checked={prefs.followSystem} onChange={(e) => update({ followSystem: e.target.checked })} />
          </Box>
        </Sheet>

        <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 3, mb: 3 }}>
          <Typography level="title-md" sx={{ mb: 1 }}>
            Sounds by area
          </Typography>
          {CATEGORIES.map((c: SoundCategory, i) => (
            <Box key={c} data-testid={`sound-cat-${c}`}>
              {i > 0 && <Divider sx={{ my: 1.5 }} />}
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2 }}>
                <Box>
                  <Typography level="title-sm">{CATEGORY_LABELS[c].label}</Typography>
                  <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                    {CATEGORY_LABELS[c].help}
                  </Typography>
                </Box>
                <Switch
                  checked={prefs.enabled[c]}
                  slotProps={{ input: { "aria-label": `${CATEGORY_LABELS[c].label} sounds` } }}
                  onChange={(e) => update({ enabled: { ...prefs.enabled, [c]: e.target.checked } })}
                />
              </Box>
              <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mt: 1 }}>
                {SOUND_IDS.filter((id) => CATEGORY_OF[id] === c).map((id) => (
                  <Button
                    key={id}
                    size="sm"
                    variant="soft"
                    color={playing === id ? "primary" : "neutral"}
                    data-testid={`sound-test-${id}`}
                    aria-label={`Play ${SOUND_LABELS[id]}`}
                    startDecorator={playing === id ? <StopIcon /> : <PlayArrowIcon />}
                    onClick={() => void test(id)}
                    sx={{ borderRadius: "xl", fontSize: "xs", minHeight: 26, py: 0 }}
                  >
                    {SOUND_LABELS[id]}
                  </Button>
                ))}
              </Box>
            </Box>
          ))}
        </Sheet>

        <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 3, mb: 3 }}>
          <Typography level="title-md" sx={{ mb: 1 }}>
            Desktop notifications
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2 }}>
            <Box>
              <Typography level="title-sm">Show alerts when this tab is in the background</Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                {perm === "unsupported"
                  ? "This browser does not support desktop notifications."
                  : perm === "denied"
                    ? "Blocked in your browser settings. Allow notifications for this site to turn this on."
                    : "Your browser asks for permission the first time you turn this on."}
              </Typography>
            </Box>
            <Switch
              checked={prefs.desktop === true && perm === "granted"}
              disabled={perm === "unsupported" || perm === "denied"}
              data-testid="sound-desktop"
              onChange={(e) => void enableDesktop(e.target.checked)}
            />
          </Box>
        </Sheet>
        <Typography level="body-xs" sx={{ opacity: 0.6, minHeight: 20 }}>
          {saved ? "Saved." : "Tone files can be replaced by an administrator, see docs/services/sounds.md."}
        </Typography>
      </Container>
    </>
  );
}

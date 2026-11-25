import { useEffect, useState } from "react";
import { Box, FormControl, FormLabel, Option, Select, Switch, Typography } from "@mui/joy";
import BlurOnIcon from "@mui/icons-material/BlurOn";
import BlockIcon from "@mui/icons-material/Block";
import {
  BG_PRESETS,
  listDevices,
  useLocalMedia,
  type BgSpec,
  type DeviceLists,
  type LocalMedia,
} from "./media";

interface Props {
  lm: LocalMedia;
  speakerId: string;
  onSpeakerChange: (id: string) => void;
  /** Light text for the dark call UI. */
  dark?: boolean;
}

/** Device pickers, noise suppression and background effects; used in green room and in call. */
export function MediaSettings({ lm, speakerId, onSpeakerChange, dark }: Props) {
  useLocalMedia(lm);
  const [devices, setDevices] = useState<DeviceLists>({
    mics: [],
    cams: [],
    speakers: [],
  });

  useEffect(() => {
    let alive = true;
    const load = () => listDevices().then((d) => alive && setDevices(d));
    void load();
    navigator.mediaDevices?.addEventListener?.("devicechange", load);
    return () => {
      alive = false;
      navigator.mediaDevices?.removeEventListener?.("devicechange", load);
    };
  }, []);

  // Labels only appear after permission, so re-list when tracks change.
  useEffect(() => {
    listDevices().then(setDevices);
  }, [lm.micTrack, lm.rawCam]);

  const label = (d: MediaDeviceInfo, i: number) => d.label || `Device ${i + 1}`;
  const bgKey =
    lm.bg.type === "none"
      ? "none"
      : lm.bg.type === "blur"
        ? "blur"
        : lm.bg.id;
  const setBg = (b: BgSpec) => void lm.setBackground(b);
  const color = dark ? "#e8eaed" : undefined;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <FormControl size="sm">
        <FormLabel sx={{ color }}>Microphone</FormLabel>
        <Select
          size="sm"
          value={lm.micId || null}
          placeholder="Default"
          onChange={(_, val) => val && void lm.switchMic(val)}
          slotProps={{ button: { "aria-label": "Microphone" } }}
        >
          {devices.mics.map((d, i) => (
            <Option key={d.deviceId} value={d.deviceId}>
              {label(d, i)}
            </Option>
          ))}
        </Select>
      </FormControl>
      <FormControl size="sm">
        <FormLabel sx={{ color }}>Camera</FormLabel>
        <Select
          size="sm"
          value={lm.camId || null}
          placeholder="Default"
          onChange={(_, val) => val && void lm.switchCam(val)}
          slotProps={{ button: { "aria-label": "Camera" } }}
        >
          {devices.cams.map((d, i) => (
            <Option key={d.deviceId} value={d.deviceId}>
              {label(d, i)}
            </Option>
          ))}
        </Select>
      </FormControl>
      {devices.speakers.length > 0 && (
        <FormControl size="sm">
          <FormLabel sx={{ color }}>Speakers</FormLabel>
          <Select
            size="sm"
            value={speakerId || null}
            placeholder="Default"
            onChange={(_, val) => val && onSpeakerChange(val)}
            slotProps={{ button: { "aria-label": "Speakers" } }}
          >
            {devices.speakers.map((d, i) => (
              <Option key={d.deviceId} value={d.deviceId}>
                {label(d, i)}
              </Option>
            ))}
          </Select>
        </FormControl>
      )}
      <FormControl
        size="sm"
        orientation="horizontal"
        sx={{ justifyContent: "space-between" }}
      >
        <Box>
          <FormLabel sx={{ color }}>Noise suppression</FormLabel>
          <Typography level="body-xs" sx={{ color: dark ? "#9aa0a6" : undefined }}>
            Browser built-in
          </Typography>
        </Box>
        <Switch
          checked={lm.noiseSuppression}
          onChange={(e) => void lm.setNoiseSuppression(e.target.checked)}
          slotProps={{ input: { "aria-label": "Noise suppression" } }}
        />
      </FormControl>

      <Box>
        <FormLabel sx={{ color, mb: 0.5 }}>Background</FormLabel>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <BgChip
            active={bgKey === "none"}
            label="None"
            onClick={() => setBg({ type: "none" })}
          >
            <BlockIcon fontSize="small" />
          </BgChip>
          <BgChip
            active={bgKey === "blur"}
            label="Blur"
            onClick={() => setBg({ type: "blur", blur: 25 })}
          >
            <BlurOnIcon fontSize="small" />
          </BgChip>
          {BG_PRESETS.map((p) => (
            <BgChip
              key={p.id}
              active={bgKey === p.id}
              label={p.label}
              onClick={() => setBg({ type: "image", src: p.src, id: p.id })}
            >
              <img
                src={p.src}
                alt=""
                style={{ width: "100%", height: "100%", objectFit: "cover" }}
              />
            </BgChip>
          ))}
        </Box>
        {lm.bgBusy && (
          <Typography level="body-xs" sx={{ mt: 0.5, color: dark ? "#9aa0a6" : undefined }}>
            Loading background effect…
          </Typography>
        )}
      </Box>
    </Box>
  );
}

function BgChip({
  active,
  label,
  onClick,
  children,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <Box
      component="button"
      type="button"
      onClick={onClick}
      aria-label={`Background: ${label}`}
      aria-pressed={active}
      title={label}
      sx={{
        width: 56,
        height: 38,
        borderRadius: "sm",
        overflow: "hidden",
        cursor: "pointer",
        p: 0,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        bgcolor: "background.level2",
        color: "text.primary",
        border: "2px solid",
        borderColor: active ? "primary.500" : "transparent",
      }}
    >
      {children}
    </Box>
  );
}

import { useEffect, useRef, useState } from "react";
import { Box, Button, Input, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";

interface Props {
  open: boolean;
  onClose: () => void;
  /** Called with the scanned or typed code. */
  onCode: (code: string) => void;
}

type BarcodeDetectorLike = {
  detect: (src: CanvasImageSource) => Promise<{ rawValue: string }[]>;
};

/** Scan with the camera where the browser has BarcodeDetector, else type the code in. */
export function ScanDialog({ open, onClose, onCode }: Props) {
  const [typed, setTyped] = useState("");
  const [camError, setCamError] = useState("");
  const video = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    if (!open) return;
    setTyped("");
    setCamError("");
    const Det = (window as unknown as {
      BarcodeDetector?: new (o: { formats: string[] }) => BarcodeDetectorLike;
    }).BarcodeDetector;
    if (!Det || !navigator.mediaDevices?.getUserMedia) {
      setCamError("Camera scanning is not supported in this browser. Type or use a USB scanner.");
      return;
    }
    let stream: MediaStream | null = null;
    let stop = false;
    const det = new Det({ formats: ["ean_13", "ean_8", "upc_a", "upc_e", "code_128", "code_39", "qr_code"] });
    navigator.mediaDevices
      .getUserMedia({ video: { facingMode: "environment" } })
      .then(async (s) => {
        stream = s;
        if (stop) return s.getTracks().forEach((t) => t.stop());
        const v = video.current;
        if (!v) return;
        v.srcObject = s;
        await v.play();
        const tick = async () => {
          if (stop) return;
          try {
            const found = await det.detect(v);
            if (found.length > 0) {
              onCode(found[0].rawValue);
              return;
            }
          } catch {
            /* keep scanning */
          }
          setTimeout(tick, 250);
        };
        tick();
      })
      .catch(() => setCamError("Camera unavailable. Type the code instead."));
    return () => {
      stop = true;
      stream?.getTracks().forEach((t) => t.stop());
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Scan barcode</Typography>
        {!camError && (
          <Box
            component="video"
            ref={video}
            muted
            playsInline
            sx={{ width: "100%", borderRadius: "md", bgcolor: "black", maxHeight: 260 }}
          />
        )}
        {camError && <Typography level="body-sm">{camError}</Typography>}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (typed.trim()) onCode(typed.trim());
          }}
        >
          <Box sx={{ display: "flex", gap: 1 }}>
            <Input
              autoFocus
              placeholder="Type or scan a barcode / SKU"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              sx={{ flex: 1 }}
              slotProps={{ input: { "aria-label": "Barcode" } }}
            />
            <Button type="submit">Find</Button>
          </Box>
        </form>
      </ModalDialog>
    </Modal>
  );
}

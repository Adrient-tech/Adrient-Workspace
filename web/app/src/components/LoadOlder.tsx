import { useState } from "react";
import { Box, Button, Typography } from "@mui/joy";

/**
 * "Load older" footer for server-paged feeds. `next` is the cursor from the
 * last reply ("" hides the button); `loadMore` fetches that page and appends.
 */
export function LoadOlder({
  next,
  shown,
  loadMore,
  label = "Load older",
  testId = "load-older",
}: {
  next: string;
  shown: number;
  loadMore: (cursor: string) => Promise<void>;
  label?: string;
  testId?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (!next) {
    return shown > 0 ? (
      <Typography level="body-xs" sx={{ p: 1, textAlign: "center", color: "text.tertiary" }}>
        {shown.toLocaleString("en-US")} shown, that is everything.
      </Typography>
    ) : null;
  }
  return (
    <Box sx={{ p: 1.5, display: "flex", gap: 1, justifyContent: "center", alignItems: "center" }}>
      <Typography level="body-xs">{shown.toLocaleString("en-US")} shown</Typography>
      <Button
        size="sm"
        variant="outlined"
        loading={busy}
        data-testid={testId}
        onClick={async () => {
          setBusy(true);
          setError("");
          try {
            await loadMore(next);
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        {label}
      </Button>
      {error && (
        <Typography level="body-xs" color="danger" role="alert">
          {error}
        </Typography>
      )}
    </Box>
  );
}

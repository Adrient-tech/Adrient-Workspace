import { useCallback, useState } from "react";
import type { ReactNode } from "react";
import { Alert, Chip, Sheet, Typography } from "@mui/joy";
import { STATUS_COLOR, STATUS_LABEL } from "./logic";
import type { DisplayStatus } from "./types";

export const rightCell = { textAlign: "right" } as const;

export function useErr() {
  const [err, setErr] = useState("");
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    try {
      setErr("");
      return await fn();
    } catch (e) {
      setErr((e as Error).message);
      return undefined;
    }
  }, []);
  return { err, setErr, run };
}

export function ErrorAlert({ err }: { err: string }) {
  return err ? (
    <Alert color="danger" role="alert" sx={{ my: 1 }}>
      {err}
    </Alert>
  ) : null;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
      <Typography level="title-md">{children}</Typography>
    </Sheet>
  );
}

export function Stat({ label, value, color, testid }: { label: string; value: string; color?: string; testid?: string }) {
  return (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", minWidth: 150, flex: "1 1 150px" }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h4" data-testid={testid} sx={{ color }}>
        {value}
      </Typography>
    </Sheet>
  );
}

export function StatusChip({ status }: { status: DisplayStatus }) {
  return (
    <Chip size="sm" variant="soft" color={STATUS_COLOR[status] ?? "neutral"} data-testid="invc-status">
      {STATUS_LABEL[status] ?? status}
    </Chip>
  );
}

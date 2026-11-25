import { useCallback, useState } from "react";
import type { ReactNode } from "react";
import { Alert, Sheet, Typography } from "@mui/joy";
import type { Me, Person } from "./api";

export interface Ctx {
  me: Me;
  people: Person[];
  reload: () => void;
}

export const personName = (people: Person[], id: string | null | undefined) => (id ? (people.find((p) => p.id === id)?.name ?? "Unknown") : "");

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
    <Sheet variant="soft" sx={{ p: 3, textAlign: "center", borderRadius: "md" }}>
      <Typography level="title-md">{children}</Typography>
    </Sheet>
  );
}

export function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, marginBottom: 8, flexWrap: "wrap" }}>
        <Typography level="title-lg">{title}</Typography>
        <div style={{ display: "flex", gap: 8 }}>{actions}</div>
      </div>
      {children}
    </Sheet>
  );
}

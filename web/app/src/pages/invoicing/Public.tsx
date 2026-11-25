import { useCallback, useEffect, useState } from "react";
import { Link as RouterLink, useParams } from "react-router-dom";
import { Alert, Box, Button, Checkbox, CircularProgress, FormControl, FormLabel, Input, Sheet, Textarea, Typography } from "@mui/joy";
import { ApiError, getPublic, publicAccept, publicDecline, publicPay } from "./api";
import type { PublicData } from "./api";
import { DocumentSheet } from "./DocumentSheet";

/** Signed-out share page at /invoice/:token: read, print, accept or decline an estimate, pay an invoice. */
export default function PublicInvoice() {
  const { token = "" } = useParams();
  const [data, setData] = useState<PublicData | null>(null);
  const [missing, setMissing] = useState(false);
  const [name, setName] = useState("");
  const [agree, setAgree] = useState(false);
  const [reason, setReason] = useState("");
  const [declining, setDeclining] = useState(false);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    getPublic(token)
      .then(setData)
      .catch(() => setMissing(true));
  }, [token]);
  useEffect(load, [load]);

  const act = async (fn: () => Promise<unknown>, done: string) => {
    setBusy(true);
    setErr("");
    try {
      await fn();
      setMsg(done);
      load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const pay = async () => {
    setBusy(true);
    setErr("");
    try {
      const r = await publicPay(token);
      window.location.href = r.url;
    } catch (e) {
      setErr(e instanceof ApiError && e.status === 409 ? "Online payment is not available for this business yet. Please use the payment details below." : (e as Error).message);
      setBusy(false);
    }
  };

  const d = data?.document;
  const canRespond = d?.kind === "estimate" && d.status === "sent" && d.display_status !== "expired";

  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "#f3f4f6", py: 3, px: 1.5 }}>
      {!data && !missing && (
        <Box sx={{ textAlign: "center", mt: 8 }}>
          <CircularProgress />
        </Box>
      )}
      {missing && (
        <Box sx={{ textAlign: "center", mt: 8 }} data-testid="invc-public-missing">
          <Typography level="h3">This link is not available</Typography>
          <Typography level="body-md">The document may have been withdrawn, or the link is wrong.</Typography>
        </Box>
      )}
      {data && d && (
        <>
          <Box className="invc-noprint" sx={{ maxWidth: 820, mx: "auto", mb: 1.5 }}>
            {d.status === "superseded" && (
              <Alert color="warning" sx={{ mb: 1 }} data-testid="invc-public-superseded">
                This estimate was replaced by a newer version.{" "}
                {d.latest_token && (
                  <RouterLink to={`/invoice/${d.latest_token}`} onClick={() => setTimeout(() => window.location.reload(), 0)}>
                    Open the latest version
                  </RouterLink>
                )}
              </Alert>
            )}
            {d.kind === "estimate" && d.display_status === "expired" && <Alert color="warning" sx={{ mb: 1 }}>This estimate expired on {d.valid_until}. Please ask for a new one.</Alert>}
            {d.status === "accepted" && d.kind === "estimate" && (
              <Alert color="success" sx={{ mb: 1 }} data-testid="invc-public-accepted">
                Accepted by {d.accepted_name}. Thank you.
              </Alert>
            )}
            {d.status === "declined" && d.kind === "estimate" && <Alert color="neutral" sx={{ mb: 1 }}>You declined this estimate.</Alert>}
            {msg && (
              <Alert color="success" sx={{ mb: 1 }} data-testid="invc-public-msg">
                {msg}
              </Alert>
            )}
            {err && (
              <Alert color="danger" sx={{ mb: 1 }} role="alert">
                {err}
              </Alert>
            )}
            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, flexWrap: "wrap" }}>
              {d.kind === "invoice" && d.balance > 0.004 && d.status !== "draft" && (
                <Button onClick={pay} loading={busy} disabled={!data.can_pay_online} data-testid="invc-public-pay" title={data.can_pay_online ? "" : "Online payment is not configured by this business"}>
                  {data.can_pay_online ? "Pay online" : "Online payment not available"}
                </Button>
              )}
              <Button variant="outlined" onClick={() => window.print()} data-testid="invc-public-print">
                Print / save as PDF
              </Button>
            </Box>
          </Box>
          <DocumentSheet doc={d} business={data.business} />
          {canRespond && (
            <Sheet className="invc-noprint" variant="outlined" sx={{ maxWidth: 820, mx: "auto", mt: 2, p: 2, borderRadius: "md" }} data-testid="invc-public-respond">
              <Typography level="title-md">Your decision</Typography>
              {!declining ? (
                <>
                  <FormControl sx={{ mt: 1 }}>
                    <FormLabel>Type your full name to sign</FormLabel>
                    <Input value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "invc-public-name", autoComplete: "name" } }} />
                  </FormControl>
                  <Checkbox sx={{ mt: 1 }} checked={agree} onChange={(e) => setAgree(e.target.checked)} label={`I accept estimate ${d.number} for ${d.currency} ${d.total.toFixed(2)} and agree to its terms.`} slotProps={{ input: { "data-testid": "invc-public-agree" } }} />
                  <Typography level="body-xs" sx={{ mt: 0.5 }}>
                    Your name, the time and your network address are saved with this acceptance.
                  </Typography>
                  <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
                    <Button color="success" loading={busy} disabled={name.trim().length < 2 || !agree} onClick={() => act(() => publicAccept(token, name, agree), "Thank you. Your acceptance was recorded.")} data-testid="invc-public-accept">
                      Accept estimate
                    </Button>
                    <Button variant="outlined" color="neutral" onClick={() => setDeclining(true)} data-testid="invc-public-decline-open">
                      Decline
                    </Button>
                  </Box>
                </>
              ) : (
                <>
                  <FormControl sx={{ mt: 1 }}>
                    <FormLabel>Reason (optional)</FormLabel>
                    <Textarea minRows={2} value={reason} onChange={(e) => setReason(e.target.value)} />
                  </FormControl>
                  <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
                    <Button color="danger" loading={busy} onClick={() => act(() => publicDecline(token, reason), "Your decision was recorded.")} data-testid="invc-public-decline">
                      Decline estimate
                    </Button>
                    <Button variant="plain" color="neutral" onClick={() => setDeclining(false)}>
                      Back
                    </Button>
                  </Box>
                </>
              )}
            </Sheet>
          )}
        </>
      )}
    </Box>
  );
}

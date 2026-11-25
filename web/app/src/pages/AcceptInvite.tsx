// Public landing page for an invitation link (/accept-invite?code=...). Shows
// who invited you, asks you to sign in with the invited address, then redeems
// the invitation. Works signed out (sign-in) and signed in (accept).
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, Box, Button, Sheet, Typography } from "@mui/joy";
import { identityApi } from "./admin/identityApi";
import { loginURL } from "../api/client";

interface Props {
  signedInEmail: string | null;
}

export default function AcceptInvite({ signedInEmail }: Props) {
  const [params] = useSearchParams();
  const code = params.get("code") ?? "";
  const [info, setInfo] = useState<{ org: string; email: string; user_type: string; apps: string[] } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  useEffect(() => {
    if (!code) {
      setError("This invitation link is incomplete.");
      return;
    }
    identityApi
      .lookupInvite(code)
      .then(setInfo)
      .catch(() => {
        // A link already redeemed by the signed-in user still counts as accepted.
        if (signedInEmail) {
          identityApi.acceptInvite(code).then(() => setDone(true)).catch(() => setError("This invitation is invalid, expired or already used."));
        } else {
          setError("This invitation is invalid, expired or already used.");
        }
      });
  }, [code, signedInEmail]);

  const accept = async () => {
    try {
      await identityApi.acceptInvite(code);
      setDone(true);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", p: 2 }}>
      <Sheet variant="outlined" sx={{ p: 4, borderRadius: "lg", maxWidth: 460, width: "100%" }} data-testid="accept-invite">
        <Typography level="h3" sx={{ mb: 1 }}>
          Workspace invitation
        </Typography>
        {error && <Alert color="danger">{error}</Alert>}
        {done && (
          <>
            <Alert color="success" sx={{ mb: 2 }}>
              Invitation accepted.
            </Alert>
            <Button component="a" href="/">
              Open workspace
            </Button>
          </>
        )}
        {info && !done && (
          <>
            <Typography level="body-md" sx={{ mb: 2 }}>
              You are invited to <b>{info.org}</b> as a {info.user_type}. Sign in as <b>{info.email}</b> to accept.
            </Typography>
            {signedInEmail ? (
              signedInEmail.toLowerCase() === info.email.toLowerCase() ? (
                <Button onClick={accept}>Accept invitation</Button>
              ) : (
                <Alert color="warning">
                  You are signed in as {signedInEmail}. Sign out and sign in as {info.email}.
                </Alert>
              )
            ) : (
              <Button component="a" href={loginURL()}>
                Sign in to accept
              </Button>
            )}
          </>
        )}
      </Sheet>
    </Box>
  );
}

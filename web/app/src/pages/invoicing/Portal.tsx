import { useEffect, useState } from "react";
import { Link as RouterLink, useParams } from "react-router-dom";
import { Box, Button, CircularProgress, Sheet, Table, Typography } from "@mui/joy";
import { getPortal } from "./api";
import type { PortalData } from "./api";
import { money } from "./logic";
import { StatusChip, rightCell } from "./ui";
import type { DisplayStatus } from "./types";

/** Signed-out customer portal at /portal/:token: every invoice and open estimate of one customer, each linking to its pay/accept page. */
export default function CustomerPortal() {
  const { token = "" } = useParams();
  const [data, setData] = useState<PortalData | null>(null);
  const [missing, setMissing] = useState(false);
  useEffect(() => {
    getPortal(token)
      .then(setData)
      .catch(() => setMissing(true));
  }, [token]);

  if (missing)
    return (
      <Box sx={{ textAlign: "center", mt: 8 }} data-testid="portal-missing">
        <Typography level="h3">This link is not available</Typography>
      </Box>
    );
  if (!data)
    return (
      <Box sx={{ textAlign: "center", mt: 8 }}>
        <CircularProgress />
      </Box>
    );
  const accent = data.business.accent || "#126C5A";
  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "#f3f4f6", py: 3, px: 1.5 }} data-testid="portal">
      <Sheet variant="outlined" sx={{ maxWidth: 820, mx: "auto", p: { xs: 2, sm: 3 }, borderRadius: "md", bgcolor: "#fff" }}>
        {data.business.logo_url && <img src={data.business.logo_url} alt="" style={{ maxHeight: 48, marginBottom: 8 }} />}
        <Typography level="h3" sx={{ color: accent }}>
          {data.business.business_name || "Your account"}
        </Typography>
        <Typography level="body-md">Account of {data.customer}</Typography>
        <Box sx={{ my: 2, p: 2, borderRadius: "md", bgcolor: "#f5f5f5" }}>
          <Typography level="body-xs">Total due</Typography>
          <Typography level="h2" data-testid="portal-due">
            {money(data.total_due, data.currency)}
          </Typography>
        </Box>
        <Typography level="title-md" sx={{ mb: 1 }}>
          Invoices
        </Typography>
        <Table size="sm" data-testid="portal-invoices">
          <thead>
            <tr>
              <th>Number</th>
              <th>Due</th>
              <th>Status</th>
              <th style={rightCell}>Total</th>
              <th style={rightCell}>Balance</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data.invoices.map((i) => (
              <tr key={i.share_token}>
                <td>{i.number}</td>
                <td>{i.due_date}</td>
                <td>
                  <StatusChip status={i.status as DisplayStatus} />
                </td>
                <td style={rightCell}>{money(i.total, i.currency)}</td>
                <td style={rightCell}>{money(i.balance, i.currency)}</td>
                <td style={rightCell}>
                  <Button size="sm" variant={i.balance > 0.004 ? "solid" : "outlined"} component={RouterLink} to={`/invoice/${i.share_token}`}>
                    {i.balance > 0.004 ? "View and pay" : "View"}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        {data.estimates.length > 0 && (
          <>
            <Typography level="title-md" sx={{ mt: 2, mb: 1 }}>
              Estimates
            </Typography>
            <Table size="sm">
              <tbody>
                {data.estimates.map((e) => (
                  <tr key={e.share_token}>
                    <td>{e.number}</td>
                    <td>
                      <StatusChip status={e.status as DisplayStatus} />
                    </td>
                    <td style={rightCell}>{money(e.total, e.currency)}</td>
                    <td style={rightCell}>
                      <Button size="sm" variant="outlined" component={RouterLink} to={`/invoice/${e.share_token}`}>
                        Review
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </>
        )}
        {(data.business.email || data.business.phone) && (
          <Typography level="body-xs" sx={{ mt: 2 }}>
            Questions? {[data.business.email, data.business.phone].filter(Boolean).join(" · ")}
          </Typography>
        )}
      </Sheet>
    </Box>
  );
}

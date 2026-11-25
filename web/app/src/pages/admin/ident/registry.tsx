// Registry of the identity admin sections. index.tsx adds them to the left nav
// and renders them; everything else about them lives in this folder.
import type { ReactNode } from "react";
import HubIcon from "@mui/icons-material/Hub";
import EventSeatIcon from "@mui/icons-material/EventSeat";
import VpnKeyIcon from "@mui/icons-material/VpnKey";
import PersonOffIcon from "@mui/icons-material/PersonOff";
import FactCheckIcon from "@mui/icons-material/FactCheck";
import ManageAccountsIcon from "@mui/icons-material/ManageAccounts";
import ManageSearchIcon from "@mui/icons-material/ManageSearch";
import NotificationsActiveIcon from "@mui/icons-material/NotificationsActive";
import { ProvisioningSection } from "./ProvisioningSection";
import { SeatsSection } from "./SeatsSection";
import { SignInSection } from "./SignInSection";
import { OffboardingSection } from "./OffboardingSection";
import { ReviewsSection } from "./ReviewsSection";
import { AdminRolesSection } from "./AdminRolesSection";
import { AuditSearchSection } from "./AuditSearchSection";
import { NotifySourcesSection } from "./NotifySourcesSection";
import { SecOpsSection } from "../secops";
import GppGoodIcon from "@mui/icons-material/GppGood";

export type IdentSectionId = "provisioning" | "seats" | "signin" | "offboarding" | "reviews" | "adminroles" | "auditsearch" | "notifysources" | "secops";

export const IDENT_SECTIONS: { id: IdentSectionId; label: string; icon: ReactNode }[] = [
  { id: "provisioning", label: "Sign-in and provisioning", icon: <HubIcon /> },
  { id: "seats", label: "Seats and licences", icon: <EventSeatIcon /> },
  { id: "signin", label: "Sign-in security", icon: <VpnKeyIcon /> },
  { id: "offboarding", label: "Offboarding", icon: <PersonOffIcon /> },
  { id: "reviews", label: "Access reviews", icon: <FactCheckIcon /> },
  { id: "adminroles", label: "Admin roles", icon: <ManageAccountsIcon /> },
  { id: "auditsearch", label: "Audit search", icon: <ManageSearchIcon /> },
  { id: "notifysources", label: "Notification sources", icon: <NotificationsActiveIcon /> },
  { id: "secops", label: "Security operations", icon: <GppGoodIcon /> },
];

export function isIdentSection(id: string): id is IdentSectionId {
  return IDENT_SECTIONS.some((s) => s.id === id);
}

export function IdentSection({ id }: { id: IdentSectionId }) {
  switch (id) {
    case "provisioning":
      return <ProvisioningSection />;
    case "seats":
      return <SeatsSection />;
    case "signin":
      return <SignInSection />;
    case "offboarding":
      return <OffboardingSection />;
    case "reviews":
      return <ReviewsSection />;
    case "adminroles":
      return <AdminRolesSection />;
    case "auditsearch":
      return <AuditSearchSection />;
    case "notifysources":
      return <NotifySourcesSection />;
    case "secops":
      return <SecOpsSection />;
  }
}

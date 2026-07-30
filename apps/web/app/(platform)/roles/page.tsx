"use client";

import { usePlatformSession } from "../../components/platform-shell";
import { RolesPage } from "../../features/roles/roles-page";

export default function Page() {
  const { user } = usePlatformSession();
  return <RolesPage defaultOrganizationId={user.organizationId} />;
}

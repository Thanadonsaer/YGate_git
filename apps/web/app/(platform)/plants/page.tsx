"use client";

import { usePlatformSession } from "../../components/platform-shell";
import { PlantsPage } from "../../features/plants/plants-page";

export default function Page() {
  const { user } = usePlatformSession();
  return <PlantsPage defaultOrganizationId={user.organizationId} />;
}

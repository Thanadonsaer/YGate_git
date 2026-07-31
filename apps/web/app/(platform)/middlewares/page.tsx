"use client";

import { usePlatformSession } from "../../components/platform-shell";
import { MiddlewaresPage } from "../../features/middlewares/middlewares-page";

export default function Page() {
  const { user } = usePlatformSession();
  return <MiddlewaresPage defaultOrganizationId={user.organizationId} />;
}

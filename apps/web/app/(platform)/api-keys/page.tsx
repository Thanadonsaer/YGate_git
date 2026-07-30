"use client";

import { usePlatformSession } from "../../components/platform-shell";
import { APIKeysPage } from "../../features/api-keys/api-keys-page";

export default function Page() {
  const { user } = usePlatformSession();
  return <APIKeysPage defaultOrganizationId={user.organizationId} />;
}

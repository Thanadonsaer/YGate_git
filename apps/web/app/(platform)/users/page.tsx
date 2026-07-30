"use client";

import { usePlatformSession } from "../../components/platform-shell";
import { UsersPage } from "../../features/users/users-page";

export default function Page() {
  const { user } = usePlatformSession();
  return <UsersPage currentUserId={user.id} defaultOrganizationId={user.organizationId} />;
}

import type { User } from "./types";

export function hasPermission(user: User, requirement: string): boolean {
  return user.permissions.includes(requirement);
}

export function can(user: User, resourceType: string, action: string): boolean {
  return hasPermission(user, `${resourceType}:${action}`);
}

// Creating an organization is a global-only permission assigned to System Admin.
export function isSystemAdmin(user: User): boolean {
  return can(user, "organization", "create");
}

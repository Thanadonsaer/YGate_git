import type { User } from "./types";

export function hasPermission(user: User, requirement: string): boolean {
  return user.permissions.includes(requirement);
}

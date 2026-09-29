import type { NavGroup, NavItem } from "../components/shell";

/**
 * The application map.
 *
 * design.md 6.1 requires the navigation to be grouped vertically with clear
 * section labels, and the event rules are explicit that role checks belong in
 * the backend. So this file decides only which groups a viewer is *offered*.
 *
 * Every route behind a group authorizes independently. Hiding a link is a
 * courtesy to the person reading the page, not a control: a viewer who types a
 * URL they were not shown is refused by the resolver, not by this list. That is
 * why `roles` here is about navigation and not about access, and why the name of
 * the field says so.
 *
 * The item and group shapes themselves live with the shell that renders them,
 * so there is one definition to change instead of two that had already begun to
 * drift and needed a cast to be used together.
 */
export type { NavGroup, NavItem };

export interface MappedGroup extends NavGroup {
  /** Which roles are offered this group. Empty means everyone, signed out included. */
  roles: string[];
}

export const navigation: MappedGroup[] = [
  {
    label: "Overview",
    roles: ["visitor", "participant", "judge", "organizer", "admin"],
    items: [{ label: "Dashboard", to: "/dashboard", icon: "layout-dashboard", end: true }],
  },
  {
    label: "Events",
    roles: ["visitor", "participant", "judge", "organizer", "admin"],
    items: [
      { label: "All events", to: "/events", icon: "calendar", end: true },
      { label: "Gallery", to: "/events/gallery", icon: "folder-kanban" },
    ],
  },
  {
    label: "My account",
    roles: ["participant", "judge", "organizer", "admin"],
    items: [
      { label: "Overview", to: "/account", icon: "user", end: true },
      { label: "Profile", to: "/account/profile", icon: "pencil" },
      { label: "Teams", to: "/account/teams", icon: "users" },
      { label: "Security", to: "/account/security", icon: "lock" },
    ],
  },
  {
    label: "Judging",
    roles: ["judge", "organizer", "admin"],
    items: [
      { label: "My assignments", to: "/judge", icon: "clipboard-list", end: true },
      { label: "Compare projects", to: "/judge/compare", icon: "git-branch" },
    ],
  },
  {
    label: "Organizing",
    roles: ["organizer", "admin"],
    items: [
      { label: "Progress", to: "/organizer", icon: "gauge", end: true },
      { label: "Submissions", to: "/organizer/submissions", icon: "folder-kanban" },
      { label: "Panel", to: "/organizer/panel", icon: "users" },
      { label: "Results", to: "/organizer/results", icon: "trophy" },
      { label: "Audit", to: "/organizer/audit", icon: "shield" },
      { label: "Grants", to: "/organizer/grants", icon: "key" },
    ],
  },
  {
    label: "Administration",
    roles: ["admin"],
    items: [
      { label: "Accounts", to: "/admin/accounts", icon: "users" },
      { label: "Platform audit", to: "/admin/audit", icon: "shield" },
      { label: "Grants", to: "/admin/grants", icon: "key" },
    ],
  },
];

/** The role a viewer must hold for a group, for a guard on the route itself. */
export function rolesForGroup(slug: string): string[] {
  const group = navigation.find((item) =>
    item.items.some((entry) => entry.to === slug || entry.to.startsWith(`${slug}/`)),
  );
  return group?.roles ?? [];
}

import { useState } from "react";
import { Route, Routes, useLocation } from "react-router-dom";

import { NavDrawer, NavGroupView, SidebarBrand, TopBar, type NavGroup } from "./components/shell";
import { useSession } from "./lib/session";
import { navigation, type NavItem } from "./lib/navigation";
import { Alert } from "./components/feedback";
import { AccountApp } from "./apps/account/AccountApp";
import { AdminApp } from "./apps/admin/AdminApp";
import { AuthApp } from "./apps/auth/AuthApp";
import { DashboardApp } from "./apps/dashboard/DashboardApp";
import { JudgeApp } from "./apps/judge/JudgeApp";
import { OrganizerApp } from "./apps/organizer/OrganizerApp";
import { PublicApp } from "./apps/public/PublicApp";

/**
 * The shell.
 *
 * design.md 5.2 and 6. The sidebar is built from the same NavGroupView the mobile
 * drawer uses, which is what 6.2 requires: one set of navigation items, two
 * containers, not two designs.
 *
 * Hiding a group is a convenience, never a control. Every route behind a group
 * authorizes independently in the backend, so a viewer who types a URL they were
 * not offered is refused by the server rather than by the sidebar having hidden
 * the link. That distinction is the difference between a UI and access control,
 * and the event rules are explicit about it.
 */
export function App() {
  const session = useSession();
  const location = useLocation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  const groups = navigation.filter((group) => group.roles.includes(session.user?.role ?? "visitor"));

  function isCurrent(item: NavItem): boolean {
    if (item.end) return location.pathname === item.to;
    return location.pathname === item.to || location.pathname.startsWith(`${item.to}/`);
  }

  // The sign-in page has no sidebar to navigate, so it renders on its own
  // rather than inside an empty shell.
  if (location.pathname === "/login" || location.pathname === "/") {
    return <PublicEntry />;
  }

  return (
    <div className={collapsed ? "shell shell--collapsed" : "shell"}>
      <a className="skip-link" href="#main">
        Skip to content
      </a>

      <SidebarBrand collapsed={collapsed} />

      <nav className="sidebar" aria-label="Primary">
        {groups.map((group) => (
          <NavGroupView key={group.label} group={group as NavGroup} isCurrent={isCurrent} />
        ))}
        <div className="sidebar__footer">
          <button
            type="button"
            className="btn btn--ghost sidebar__collapse"
            onClick={() => setCollapsed((value) => !value)}
            aria-expanded={!collapsed}
          >
            Collapse sidebar
          </button>
        </div>
      </nav>

      <TopBar
        title={titleFor(location.pathname)}
        onOpenDrawer={() => setDrawerOpen(true)}
        signedIn={Boolean(session.user)}
        displayName={session.user?.display_name}
        role={session.user?.role}
        onSignOut={() => void session.signOut()}
      />

      <main className="main" id="main" tabIndex={-1}>
        <div className="main__inner">
          {session.error ? (
            <Alert kind="error" title="The portal could not be reached">
              {session.error.message} This is a connection problem rather than a refusal: the
              backend answered with something unexpected.
            </Alert>
          ) : null}
          <Routes>
            <Route path="/dashboard" element={<DashboardApp />} />
            <Route path="/account/*" element={<AccountApp />} />
            <Route path="/events/*" element={<PublicApp />} />
            <Route path="/judge/*" element={<JudgeApp />} />
            <Route path="/organizer/*" element={<OrganizerApp />} />
            <Route path="/admin/*" element={<AdminApp />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </div>
      </main>

      <NavDrawer
        open={drawerOpen}
        groups={groups as NavGroup[]}
        isCurrent={isCurrent}
        onClose={() => setDrawerOpen(false)}
      />
    </div>
  );
}

/** The landing page and sign-in, which sit outside the shell. */
function PublicEntry() {
  const location = useLocation();
  if (location.pathname === "/login") return <AuthApp />;
  return <PublicApp />;
}

function NotFound() {
  return (
    <div className="stack stack-4">
      <h2 className="page-head__title">No such page</h2>
      <Alert kind="info" title="That address does not match anything here">
        The link may be from an older version of the portal, or the page may have moved.
      </Alert>
    </div>
  );
}

function titleFor(pathname: string): string {
  if (pathname.startsWith("/account")) return "My account";
  if (pathname.startsWith("/events")) return "Events";
  if (pathname.startsWith("/judge")) return "Judging";
  if (pathname.startsWith("/organizer")) return "Organizing";
  if (pathname.startsWith("/admin")) return "Administration";
  return "Dashboard";
}

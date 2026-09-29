import { useEffect, useRef, useState } from "react";
import { Route, Routes, useLocation } from "react-router-dom";

import {
  NavDrawer,
  NavGroupView,
  PublicHeader,
  TopBar,
} from "./components/shell";
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
  // Collapsed by default. A 264px navigation rail beside a 1440px content panel
  // spends a fifth of the width on labels for four or five destinations per
  // group; the rail is there for when someone wants the labels, and the icon
  // plus its accessible name is enough to navigate with. The choice is saved so
  // a reader who prefers the labels does not re-open the rail on every page.
  const [collapsed, setCollapsed] = useState(
    () => window.localStorage.getItem("codeceremony.rail") !== "expanded",
  );

  const groups = navigation.filter((group) => group.roles.includes(session.user?.role ?? "visitor"));

  function isCurrent(item: NavItem): boolean {
    if (item.end) return location.pathname === item.to;
    return location.pathname === item.to || location.pathname.startsWith(`${item.to}/`);
  }

  useRouteEffects(location.pathname);

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

      {/* The header sits above the body row, in that order: `.shell` is a flex
          column, so a TopBar rendered after `.shell__body` lands at the bottom
          of the viewport rather than at the top of it. */}
      <TopBar
        title={titleFor(location.pathname)}
        onOpenDrawer={() => setDrawerOpen(true)}
        onToggleRail={() =>
          setCollapsed((value) => {
            window.localStorage.setItem("codeceremony.rail", value ? "expanded" : "collapsed");
            return !value;
          })
        }
        railCollapsed={collapsed}
        signedIn={Boolean(session.user)}
        displayName={session.user?.display_name}
        role={session.user?.role}
        onSignOut={() => void session.signOut()}
      />

      {/* The body row. The sidebar is a column beside the content panel rather
          than a layer on top of it: a floating rail overlaps the panel's
          rounded edge, and two frosted surfaces intersecting reads as a
          rendering fault rather than as depth. */}
      <div className="shell__body">
        <nav className={collapsed ? "sidebar sidebar--collapsed" : "sidebar"} aria-label="Primary">
          <div className="sidebar__scroll">
            {groups.map((group) => (
              <NavGroupView key={group.label} group={group} isCurrent={isCurrent} />
            ))}
          </div>
        </nav>

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
              {/* Every project card in the gallery links here. With no route,
                  the click landed on the app-level not-found, so a public
                  visitor who clicked "Details" got a dead end. */}
              <Route path="/projects/:projectID" element={<PublicApp />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </div>
        </main>
      </div>

      <NavDrawer
        open={drawerOpen}
        groups={groups}
        isCurrent={isCurrent}
        onClose={() => setDrawerOpen(false)}
      />
    </div>
  );
}

/**
 * What has to happen when the address changes, and only then.
 *
 * A single-page app never tells the browser it has navigated, so without this
 * every route shares the tab title the bundle was built with, and focus stays
 * on whichever link was clicked — which, for a screen reader, means the new
 * page is never announced at all. Both are corrected here rather than in each
 * app, because a page that forgets is the normal case.
 *
 * The first render is excluded from the focus move: arriving at a deep link and
 * having focus pulled away from the document is a jolt on load, and the browser
 * is already at the top of the page.
 */
function useRouteEffects(pathname: string): void {
  const first = useRef(true);
  useEffect(() => {
    const title = titleFor(pathname);
    document.title = title === "Dashboard" ? "CodeCeremony" : `${title} · CodeCeremony`;
    if (first.current) {
      first.current = false;
      return;
    }
    // Focus moves into the main region so the route change is announced from
    // the content rather than from the link that caused it.
    document.getElementById("main")?.focus({ preventScroll: true });
  }, [pathname]);
}

/** The landing page and sign-in, which sit outside the shell. */
function PublicEntry() {
  const location = useLocation();
  const session = useSession();
  const signIn = location.pathname === "/login";
  return (
    <div className="public-shell">
      <PublicHeader
        signedIn={Boolean(session.user)}
        displayName={session.user?.display_name}
        role={session.user?.role}
        onSignOut={() => void session.signOut()}
        heading={!signIn}
      />
      <main className="main public-shell__main" id="main" tabIndex={-1}>
        <div className="main__inner">
          {session.error ? (
            <Alert kind="error" title="The portal could not be reached">
              {session.error.message}
            </Alert>
          ) : null}
          {signIn ? <AuthApp /> : <PublicApp />}
        </div>
      </main>
    </div>
  );
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

export function titleFor(pathname: string): string {
  if (pathname.startsWith("/account")) return "My account";
  if (pathname.startsWith("/events")) return "Events";
  if (pathname.startsWith("/projects")) return "Project";
  if (pathname.startsWith("/judge")) return "Judging";
  if (pathname.startsWith("/organizer")) return "Organizing";
  if (pathname.startsWith("/admin")) return "Administration";
  if (pathname.startsWith("/login")) return "Sign in";
  return "Dashboard";
}

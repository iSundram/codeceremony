import { useEffect, useRef, type ReactNode } from "react";
import { Link } from "react-router-dom";

import { IconButton } from "./controls";
import { Icon, type IconName } from "../lib/icons";

/**
 * App shell components.
 *
 * design.md 5.2 fixes the layout and 6 fixes the sidebar. The mobile drawer is
 * the same navigation items in a different container, which is what 6.2 asks
 * for: the same item components, not a second mobile navigation design.
 *
 * The drawer traps focus while open, closes on Escape, and returns focus to the
 * trigger that opened it. That is 6.2 behaviour, and a portal that a judge uses
 * on a phone in a venue is exactly where a focus-trapping dialog earns its keep.
 *
 * Every navigational element here is a react-router `Link`, never a bare `<a>`.
 * A bare anchor on a client-side route is a full document load: it drops the
 * state the viewer had, re-fires every request the page had settled, and leaves
 * the browser tab title unchanged — so the sidebar and the tab strip looked
 * like part of an app while behaving like a server-rendered site, next to
 * buttons that really did navigate in place.
 */

/**
 * One item of navigation.
 *
 * The application map in `lib/navigation.ts` extends this with the roles a
 * group is *offered* to; the shell only ever needs the destination, which is
 * why the roles live with the map rather than here. Having one definition of
 * the item is what removes the `as NavGroup` casts that used to sit between
 * the two files.
 */
export interface NavItem {
  label: string;
  to: string;
  icon: IconName;
  /** Exact path or prefix match, so a sub-page keeps its group open. */
  match?: "exact" | "prefix";
  end?: boolean;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

// ---- 7.2 SidebarNavGroup and SidebarNavItem -------------------------------

/**
 * design.md 8.4: height at least 44px, 12px radius, 12px icon-to-label gap, and
 * the active state carried by fill, weight and aria-current rather than colour
 * alone. The `nav-item--active` class is applied by the caller from the router's
 * location so the same rule is used in the sidebar and the drawer.
 */
export function NavGroupView({
  group,
  isCurrent,
  onNavigate,
}: {
  group: NavGroup;
  isCurrent: (item: NavItem) => boolean;
  onNavigate?: () => void;
}) {
  return (
    <div className="nav-group">
      <p className="nav-group__label">{group.label}</p>
      <ul className="nav-list">
        {group.items.map((item) => {
          const current = isCurrent(item);
          return (
            <li key={item.to}>
              <Link
                className={current ? "nav-item nav-item--active" : "nav-item"}
                to={item.to}
                aria-current={current ? "page" : undefined}
                onClick={onNavigate}
              >
                <span className="nav-item__icon">
                  <Icon name={item.icon} size={20} />
                </span>
                <span className="nav-item__label">{item.label}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** design.md 6.1. The brand lockup, with clear space around the mark. */
export function SidebarBrand({ collapsed = false }: { collapsed?: boolean }) {
  // design.md 10.2: the full lockup and the mark alone are not interchangeable.
  // The collapsed 84px slot gets the mark, because a wordmark does not fit and
  // shrinking it would make it illegible rather than compact.
  return (
    <Link className="sidebar-brand" to="/dashboard">
      {collapsed ? (
        <img className="sidebar-brand__mark" src="/brand/icon.svg" alt="CodeCeremony" width={28} height={28} />
      ) : (
        <img className="sidebar-brand__lockup" src="/brand/logo.svg" alt="CodeCeremony" height={24} />
      )}
    </Link>
  );
}

/** design.md 6.3. The top bar, with only the approved slots in it. */
export function TopBar({
  breadcrumb,
  title,
  actions,
  onOpenDrawer,
  onToggleRail,
  railCollapsed = true,
  signedIn,
  displayName,
  role,
  onSignOut,
}: {
  breadcrumb?: ReactNode;
  title: string;
  actions?: ReactNode;
  onOpenDrawer: () => void;
  /** Expands the rail between its collapsed and full widths. */
  onToggleRail?: () => void;
  railCollapsed?: boolean;
  signedIn: boolean;
  displayName?: string;
  role?: string;
  onSignOut: () => void;
}) {
  return (
    <header className="topbar">
      <IconButton
        icon="menu"
        label="Open navigation"
        // §6.3 allows a navigation trigger in the top bar and nothing else, and
        // forbids a second navigation system: the stylesheet hides this on every
        // width where the sidebar is persistent, so it is a mobile affordance
        // only.
        className="topbar__nav-trigger"
        onClick={onOpenDrawer}
      />

      {/* The rail toggle. The sidebar is a persistent rail on desktop, and this
          is what expands it to its full width. It is a layout control, not a
          navigation control, which is why it is the one thing in the header
          ahead of the page context. */}
      <IconButton
        icon="panel-left"
        label={railCollapsed ? "Expand the navigation rail" : "Collapse the navigation rail"}
        className="topbar__rail-toggle"
        onClick={onToggleRail}
        aria-pressed={!railCollapsed}
      />

      {/* The lockup lives in the header now. When the sidebar was a grid column
          the brand had a cell of its own above it; as a floating rail there is
          nothing above it, and a brand mark floating in a corner of a rounded
          panel is the one thing that makes the chrome look assembled rather than
          designed. §6.1 asks for the lockup at the top with clear space, and the
          header is now the top. */}
      <Link className="topbar__brand" to="/dashboard">
        <img src="/brand/icon.svg" alt="" width={56} height={56} />
        <span className="topbar__brand-name">CodeCeremony</span>
      </Link>

      <div className="topbar__divider" />

      <div className="topbar__context">
        {breadcrumb ? <div className="topbar__crumb">{breadcrumb}</div> : null}
        <h1 className="topbar__title">{title}</h1>
      </div>

      <div className="topbar__spacer" />

      <div className="topbar__actions">
        {actions ? (
          <>
            {actions}
            <div className="topbar__divider" />
          </>
        ) : null}
        {signedIn ? (
          <div className="topbar__account">
            <span className="topbar__identity">
              <span className="topbar__account-name">{displayName}</span>
              <span className="topbar__account-role">{role}</span>
            </span>
            <IconButton icon="log-out" label="Sign out" onClick={onSignOut} />
          </div>
        ) : (
          <Link className="btn btn--primary" to="/login">
            Sign in
          </Link>
        )}
      </div>
    </header>
  );
}

/** design.md 7.2. */
export function Breadcrumbs({ items }: { items: { label: string; to?: string }[] }) {
  return (
    <nav className="breadcrumbs" aria-label="Breadcrumb">
      <ol className="breadcrumbs__list">
        {items.map((item, index) => (
          <li key={`${item.label}-${index}`} className="breadcrumbs__item">
            {item.to ? (
              <Link to={item.to} className="breadcrumbs__link">
                {item.label}
              </Link>
            ) : (
              <span aria-current="page" className="breadcrumbs__current">
                {item.label}
              </span>
            )}
          </li>
        ))}
      </ol>
    </nav>
  );
}

/**
 * design.md 6.2: the mobile drawer. Same items, overlay backdrop from
 * color.overlay, Escape closes, focus returns to the trigger.
 */
export function NavDrawer({
  open,
  groups,
  isCurrent,
  onClose,
}: {
  open: boolean;
  groups: NavGroup[];
  isCurrent: (item: NavItem) => boolean;
  onClose: () => void;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const trigger = useRef<Element | null>(null);
  // The caller's close handler is read through a ref so the effect below runs
  // when `open` changes and on no other render. A fresh arrow from the caller
  // used to be in the dependency array, so every parent re-render re-ran the
  // effect: it re-captured `document.activeElement` from wherever focus was at
  // that moment and pushed focus back to the first control in the drawer,
  // pulling it out of whatever the viewer was operating.
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!open) return;
    trigger.current = document.activeElement;

    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        closeRef.current();
        return;
      }
      // Tab is wrapped so focus cannot leave the drawer and land on the page
      // behind it, which is still rendered and still focusable.
      if (event.key !== "Tab" || !panel.current) return;
      const focusable = panel.current.querySelectorAll<HTMLElement>("a[href], button");
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKey);
    const firstLink = panel.current?.querySelector<HTMLElement>("a[href], button");
    firstLink?.focus();

    return () => {
      document.removeEventListener("keydown", onKey);
      // Focus returns to whatever opened the drawer, per 6.2.
      (trigger.current as HTMLElement | null)?.focus();
    };
  }, [open]);

  if (!open) return null;

  return (
    <div className="drawer-layer">
      <div className="drawer-backdrop" onClick={onClose} aria-hidden="true" />
      <div
        className="drawer"
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label="Navigation"
      >
        <div className="drawer__head">
          <img className="sidebar-brand__lockup" src="/brand/logo.svg" alt="CodeCeremony" height={44} />
          <IconButton icon="x" label="Close navigation" onClick={onClose} />
        </div>
        <div className="drawer__body">
          {groups.map((group) => (
            <NavGroupView key={group.label} group={group} isCurrent={isCurrent} onNavigate={onClose} />
          ))}
        </div>
      </div>
    </div>
  );
}

/**
 * The page heading block, so the vertical rhythm is defined once. design.md
 * 5.2: page rhythm begins 32px after the top bar.
 */
export function PageHead({
  title,
  lede,
  actions,
}: {
  title: string;
  lede?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="page-head">
      <div className="page-head__text">
        <h2 className="page-head__title">{title}</h2>
        {lede ? <p className="page-head__lede">{lede}</p> : null}
      </div>
      {actions ? <div className="page-head__actions">{actions}</div> : null}
    </div>
  );
}

/** design.md 7.2. A link that reads as a tab, with aria-current for the state. */
export function TabLink({
  to,
  label,
  active,
  icon,
}: {
  to: string;
  label: string;
  active: boolean;
  icon?: IconName;
}) {
  return (
    <Link
      className={active ? "tab tab--active" : "tab"}
      to={to}
      aria-current={active ? "page" : undefined}
    >
      {icon ? <Icon name={icon} size={16} /> : null}
      {label}
    </Link>
  );
}

/**
 * The tab strip. It carries a label because several labelled landmarks already
 * exist on a page — sidebar, pagination, breadcrumbs — and a `<nav>` with no
 * name is a landmark a screen reader can only enumerate as "navigation".
 */
export function Tabs({ children, label = "Sections" }: { children: ReactNode; label?: string }) {
  return (
    <nav className="tabs" aria-label={label}>
      {children}
    </nav>
  );
}

/** design.md 7.2. Pages are known, so paging is explicit rather than a scroller. */
export function Pagination({
  page,
  pageCount,
  onPage,
}: {
  page: number;
  pageCount: number;
  onPage: (page: number) => void;
}) {
  if (pageCount <= 1) return null;
  return (
    <nav className="pagination" aria-label="Pagination">
      <button
        type="button"
        className="btn btn--tertiary"
        onClick={() => onPage(page - 1)}
        disabled={page <= 1}
      >
        Previous
      </button>
      <span className="pagination__status">
        Page {page} of {pageCount}
      </span>
      <button
        type="button"
        className="btn btn--tertiary"
        onClick={() => onPage(page + 1)}
        disabled={page >= pageCount}
      >
        Next
      </button>
    </nav>
  );
}

/**
 * The header for the two surfaces that sit outside the application shell: the
 * landing page at `/` and the sign-in page.
 *
 * It exists because those pages had no way in. The shell's top bar carries the
 * only "Sign in" control in the product, and `/` deliberately renders without a
 * shell, so a first-time visitor to the portal — or a signed-in person who had
 * bookmarked the gallery — reached a page with no sign-in, no dashboard and no
 * navigation of any kind, in 646 lines of otherwise complete public UI.
 *
 * `heading` decides whether the lockup is the document's `<h1>`. The sign-in
 * page owns its own heading ("Sign in to your portal"), so it passes false and
 * avoids two competing top-level headings on one screen.
 */
export function PublicHeader({
  signedIn,
  displayName,
  role,
  onSignOut,
  heading = true,
}: {
  signedIn: boolean;
  displayName?: string;
  role?: string;
  onSignOut: () => void;
  heading?: boolean;
}) {
  const lockup = (
    <img className="public-header__logo" src="/brand/logo.svg" alt="CodeCeremony" height={30} />
  );
  return (
    <header className="public-header">
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      {heading ? (
        <h1 className="public-header__brand">{lockup}</h1>
      ) : (
        <span className="public-header__brand">{lockup}</span>
      )}
      <nav className="public-header__nav" aria-label="Primary">
        <Link className="public-header__link" to="/events">
          All events
        </Link>
        <Link className="public-header__link" to="/events/gallery">
          Gallery
        </Link>
      </nav>
      <div className="public-header__actions">
        {signedIn ? (
          <>
            <span className="topbar__account-name">{displayName}</span>
            <span className="topbar__account-role">{role}</span>
            <button type="button" className="btn btn--ghost" onClick={onSignOut}>
              <Icon name="log-out" size={16} />
              Sign out
            </button>
          </>
        ) : (
          <Link className="btn btn--primary" to="/login">
            Sign in
          </Link>
        )}
      </div>
    </header>
  );
}

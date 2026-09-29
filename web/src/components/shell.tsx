import { useEffect, useRef, type ReactNode } from "react";

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
              <a
                className={current ? "nav-item nav-item--active" : "nav-item"}
                href={item.to}
                aria-current={current ? "page" : undefined}
                onClick={onNavigate}
              >
                <span className="nav-item__icon">
                  <Icon name={item.icon} size={20} />
                </span>
                <span className="nav-item__label">{item.label}</span>
              </a>
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
    <a className="sidebar-brand" href="/dashboard">
      {collapsed ? (
        <img className="sidebar-brand__mark" src="/brand/icon.svg" alt="CodeCeremony" width={28} height={28} />
      ) : (
        <img className="sidebar-brand__lockup" src="/brand/logo.svg" alt="CodeCeremony" height={24} />
      )}
    </a>
  );
}

/** design.md 6.3. The top bar, with only the approved slots in it. */
export function TopBar({
  breadcrumb,
  title,
  actions,
  onOpenDrawer,
  signedIn,
  displayName,
  role,
  onSignOut,
}: {
  breadcrumb?: ReactNode;
  title: string;
  actions?: ReactNode;
  onOpenDrawer: () => void;
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
        className="topbar__menu"
        onClick={onOpenDrawer}
      />
      <div className="topbar__context">
        {breadcrumb ? <div className="topbar__crumbs">{breadcrumb}</div> : null}
        <h1 className="topbar__title">{title}</h1>
      </div>
      <div className="topbar__spacer" />
      <div className="topbar__actions">
        {actions}
        {signedIn ? (
          <div className="topbar__account">
            <span className="topbar__account-name">{displayName}</span>
            <span className="topbar__account-role">{role}</span>
            <button type="button" className="btn btn--ghost" onClick={onSignOut}>
              <Icon name="log-out" size={16} />
              Sign out
            </button>
          </div>
        ) : (
          <a className="btn btn--primary" href="/login">
            Sign in
          </a>
        )}
      </div>
    </header>
  );
}

/** design.md 7.2. */
export function Breadcrumbs({ items }: { items: { label: string; href?: string }[] }) {
  return (
    <nav className="breadcrumbs" aria-label="Breadcrumb">
      <ol className="breadcrumbs__list">
        {items.map((item, index) => (
          <li key={`${item.label}-${index}`} className="breadcrumbs__item">
            {item.href ? (
              <a href={item.href} className="breadcrumbs__link">
                {item.label}
              </a>
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

  useEffect(() => {
    trigger.current = document.activeElement;
    if (!open) return;

    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
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
  }, [open, onClose]);

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
          <img className="sidebar-brand__lockup" src="/brand/logo.svg" alt="CodeCeremony" height={22} />
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
      <div>
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
    <a className={active ? "tab tab--active" : "tab"} href={to} aria-current={active ? "page" : undefined}>
      {icon ? <Icon name={icon} size={16} /> : null}
      {label}
    </a>
  );
}

export function Tabs({ children }: { children: ReactNode }) {
  return <nav className="tabs">{children}</nav>;
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

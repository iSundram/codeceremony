import type { ReactNode } from "react";

import { Icon, type IconName } from "../lib/icons";

/**
 * Feedback, status and data-display components.
 *
 * design.md 10.3 is the rule that shapes most of this file: there is no approved
 * status or destructive palette, so every state here is carried by words and an
 * approved glyph, never by a colour. That is why a Badge or an Alert always has
 * a label, and why the variant names below describe emphasis rather than
 * severity. There is no `error` red here, and there will not be one added later
 * without a palette amendment.
 */

const BADGE_VARIANTS = ["", "outline", "accent", "large"] as const;
export type BadgeVariant = (typeof BADGE_VARIANTS)[number];

/** design.md 7.4. The label is the point; the icon is supplementary. */
export function Badge({
  children,
  icon,
  variant = "",
}: {
  children: ReactNode;
  icon?: IconName;
  variant?: BadgeVariant;
}) {
  const className = variant ? `badge badge--${variant}` : "badge";
  return (
    <span className={className}>
      {icon ? <Icon name={icon} size={16} /> : null}
      {children}
    </span>
  );
}

/** design.md 7.4. A dot or glyph plus a word, never a coloured word alone. */
export function Status({
  children,
  icon,
  muted = false,
}: {
  children: ReactNode;
  icon?: IconName;
  muted?: boolean;
}) {
  const className = muted ? "status status--muted" : "status";
  return (
    <span className={className}>
      {icon ? <Icon name={icon} size={16} /> : <span className="status__dot" aria-hidden="true" />}
      {children}
    </span>
  );
}

// alertKinds maps the four outcomes the portal reports to their approved glyph.
// The words in the title carry the meaning; the glyph is the fast read.
const ALERT_KINDS = {
  info: "info",
  success: "circle-check",
  warning: "alert-triangle",
  error: "circle-alert",
  neutral: "info",
} as const;

export type AlertKind = keyof typeof ALERT_KINDS;

/**
 * design.md 8.7: an Alert uses text and an approved icon. Introducing red, green
 * or orange here is exactly the unapproved-colour move section 1.1 rule 3
 * forbids, so a failure is a different glyph and a different word, not a
 * different hue.
 */
export function Alert({
  kind = "info",
  title,
  children,
  actions,
}: {
  kind?: AlertKind;
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
}) {
  const className = kind === "info" || kind === "neutral" ? "alert alert--muted" : "alert";
  return (
    <div className={className} role={kind === "error" ? "alert" : "status"}>
      <span className="alert__icon">
        <Icon name={ALERT_KINDS[kind]} size={20} />
      </span>
      <div className="alert__body">
        <p className="alert__title">{title}</p>
        {children ? <div className="alert__text">{children}</div> : null}
        {actions ? <div className="alert__actions">{actions}</div> : null}
      </div>
    </div>
  );
}

export interface ProgressProps {
  label: string;
  value: number;
  total: number;
  /** The secondary fill, for a two-series comparison. Always labelled. */
  secondary?: boolean;
}

/**
 * design.md 8.8: color.accent on color.surface-muted, and the numeric value is
 * written out beside the bar. A bar alone is unreadable to a screen reader and
 * ambiguous to a person comparing two of them, so the number is part of the
 * component rather than something the caller remembers to add.
 */
export function Progress({ label, value, total, secondary = false }: ProgressProps) {
  const safeTotal = total > 0 ? total : 1;
  const clamped = Math.max(0, Math.min(value, safeTotal));
  const percent = Math.round((clamped / safeTotal) * 100);
  return (
    <div className={secondary ? "progress progress--secondary" : "progress"}>
      <div className="progress__head">
        <span className="progress__label">{label}</span>
        <span className="progress__value">
          {clamped} of {safeTotal}
        </span>
      </div>
      <div
        className="progress__track"
        role="progressbar"
        aria-valuenow={clamped}
        aria-valuemin={0}
        aria-valuemax={safeTotal}
        aria-label={label}
      >
        <div className="progress__fill" style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}

// design.md 8.8: Skeleton uses surface-muted and the border colour, and must not
// shimmer continuously by default. So there is no animation here at all.
export function Skeleton({ lines = 3 }: { lines?: number }) {
  return (
    <div className="stack stack-3">
      {Array.from({ length: lines }, (_, index) => (
        // Each bar is hidden individually rather than the wrapper, so a
        // consumer that renders one bar on its own still hides it.
        <div key={index} className="skeleton" aria-hidden="true" />
      ))}
    </div>
  );
}

/** design.md 12: every data surface needs a clear empty state. */
export function EmptyState({
  title,
  body,
  icon = "folder-kanban",
  action,
}: {
  title: string;
  body: string;
  icon?: IconName;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <span className="empty-state__icon">
        <Icon name={icon} size={24} />
      </span>
      <p className="empty-state__title">{title}</p>
      <p className="empty-state__text">{body}</p>
      {action ? <div className="alert__actions">{action}</div> : null}
    </div>
  );
}

export function ErrorState({
  title = "Something went wrong",
  body,
  retry,
}: {
  title?: string;
  body: string;
  retry?: ReactNode;
}) {
  return (
    <div className="error-state" role="alert">
      <span className="error-state__icon">
        <Icon name="circle-alert" size={24} />
      </span>
      <p className="error-state__title">{title}</p>
      <p className="error-state__text">{body}</p>
      {retry ? <div className="alert__actions">{retry}</div> : null}
    </div>
  );
}

/** design.md 8.8: loading must not cause layout shift, hence a fixed bar. */
export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <div className="stack stack-3" role="status" aria-live="polite">
      <span className="visually-hidden">{label}</span>
      <Progress label={label} value={1} total={3} />
    </div>
  );
}

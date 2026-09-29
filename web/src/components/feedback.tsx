import { useEffect, useId, useRef, type ReactNode } from "react";

import { Icon, type IconName } from "../lib/icons";
import { IconButton } from "./controls";

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
//
// The wrapper is a live region: every bar is aria-hidden, so without this a
// loading surface was silent to assistive technology — the screen reader stayed
// on the previous page's content while the sighted reader watched bars arrive.
// `LoadingState` already carried a status role and was used nowhere; putting it
// on the component every page actually renders means the fix cannot be forgot
// at the call site.
export function Skeleton({ lines = 3, label = "Loading" }: { lines?: number; label?: string }) {
  return (
    <div className="stack stack-3" role="status" aria-live="polite">
      <span className="visually-hidden">{label}</span>
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
      {/* A heading, not a paragraph: several surfaces render an empty state as
          the whole page — an unknown admin route, a judge with no assignments —
          and a page whose only title was a `<p>` had nothing to land on after
          the route change moved focus into main. */}
      <h2 className="empty-state__title">{title}</h2>
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
      <h2 className="error-state__title">{title}</h2>
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

// ---- 7.4 Modal ------------------------------------------------------------

/** The controls the focus trap walks. Inputs are in the list because a
 * confirmation can ask for a reason to be typed, and a field is focusable like
 * any other control. */
const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export interface ModalProps {
  open: boolean;
  /** Required. §13: a dialog with no name is a dialog nobody can find again. */
  title: string;
  children?: ReactNode;
  /** The footer. §10.3 puts a destructive action here, apart from the primary. */
  actions?: ReactNode;
  onClose: () => void;
  /** The accessible name of the close control, and the words on nothing. */
  closeLabel?: string;
}

/**
 * design.md 7.4 and 8.6. The confirmation surface for the two irreversible
 * actions the portal has — revoking a grant, and locking a submitted review —
 * because §10.3 requires a confirmation Modal for a destructive action and §12
 * allows confirmation for nothing else.
 *
 * The dialog is a div with role="dialog" rather than a native <dialog>, so that
 * the backdrop can be a clickable sibling and the focus trap is explicit. The
 * trap is the same one the navigation drawer uses: Tab is wrapped at both ends
 * so focus cannot reach the page behind, which is still rendered and still
 * focusable, and focus returns to whatever opened the dialog on close. A
 * confirmation a keyboard user can tab out of is not a confirmation.
 */
export function Modal({ open, title, children, actions, onClose, closeLabel = "Close" }: ModalProps) {
  const panel = useRef<HTMLDivElement>(null);
  const trigger = useRef<Element | null>(null);
  const titleId = useId();
  // Read through a ref so the effect below depends on `open` alone. Callers
  // pass `() => setConfirming(false)`, a new function on every render; with it
  // in the dependency array the trap re-ran on each render, re-capturing
  // `document.activeElement` from wherever focus was and re-focusing the first
  // control in the dialog — so a button that set a busy flag could have focus
  // pulled out of it mid-request, and the element remembered as "the trigger"
  // became a control inside the dialog, so closing never restored focus at all.
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
      if (event.key !== "Tab" || !panel.current) return;
      const focusable = panel.current.querySelectorAll<HTMLElement>(FOCUSABLE);
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
    // Focus lands inside rather than staying on the control behind, which is
    // behind the backdrop and, to a screen reader, still the current context.
    const target = panel.current?.querySelector<HTMLElement>(FOCUSABLE) ?? panel.current;
    target?.focus();

    return () => {
      document.removeEventListener("keydown", onKey);
      (trigger.current as HTMLElement | null)?.focus();
    };
  }, [open]);

  if (!open) return null;

  return (
    <div className="modal-layer">
      <div className="modal-backdrop" onClick={onClose} aria-hidden="true" />
      <div
        className="modal"
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <div className="modal__head">
          <h2 className="modal__title" id={titleId}>
            {title}
          </h2>
          <IconButton icon="x" label={closeLabel} onClick={onClose} />
        </div>
        {children ? <div className="modal__body">{children}</div> : null}
        {actions ? <div className="modal__actions">{actions}</div> : null}
      </div>
    </div>
  );
}


import type { ReactNode } from "react";

import { Progress } from "./feedback";
import { Icon, type IconName } from "../lib/icons";

/**
 * Data-display components.
 *
 * design.md 10.4 decides there is no chart component: the section 7 inventory
 * has no room for one, and adding it would be inventing a visual pattern. So a
 * distribution is a table with a Progress in a cell, a rate is a StatCard, and a
 * ranking is a Table with a right-aligned numeric column. That constraint is
 * deliberate and it is why there is nothing scatter-shaped in this file.
 */

// ---- 8.3 Cards and panels ------------------------------------------------

const CARD_TONES = ["surface", "muted", "warm", "brand"] as const;
export type CardTone = (typeof CARD_TONES)[number];

export function Card({
  children,
  tone = "surface",
  interactive = false,
  className = "",
}: {
  children: ReactNode;
  tone?: CardTone;
  interactive?: boolean;
  className?: string;
}) {
  const classes = ["card"];
  if (tone !== "surface") classes.push(`card--${tone}`);
  if (interactive) classes.push("card--interactive");
  if (className) classes.push(className);
  return <section className={classes.join(" ")}>{children}</section>;
}

export function CardTitle({ children }: { children: ReactNode }) {
  return <h3 className="card__title">{children}</h3>;
}

export function CardBody({ children }: { children: ReactNode }) {
  return <p className="card__lede">{children}</p>;
}

export function CardActions({ children }: { children: ReactNode }) {
  return <div className="card__actions">{children}</div>;
}

/**
 * design.md 7.5. The value is display type with tabular figures so a row of
 * cards aligns on the digits, which matters when the numbers are being compared
 * rather than read one at a time.
 */
export function StatCard({
  label,
  value,
  meta,
  icon,
}: {
  label: string;
  value: ReactNode;
  meta?: string;
  icon?: IconName;
}) {
  return (
    <div className="stat-card">
      <p className="stat-card__label">{label}</p>
      <p className="stat-card__value">{value}</p>
      {meta || icon ? (
        <p className="stat-card__meta">
          {icon ? <Icon name={icon} size={16} /> : null}
          {meta}
        </p>
      ) : null}
    </div>
  );
}

/** A row of StatCards that reflows rather than wrapping into a ragged wall. */
export function StatGrid({ children }: { children: ReactNode }) {
  return <div className="stat-grid">{children}</div>;
}

/** design.md 7.5. */
export function Tag({ children }: { children: ReactNode }) {
  return <span className="tag">{children}</span>;
}

export function Avatar({
  name,
  size = "sm",
}: {
  name: string;
  size?: "sm" | "lg";
}) {
  return (
    <span className={`avatar avatar--${size}`} aria-hidden="true">
      {initials(name)}
    </span>
  );
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) {
    const only = parts[0] ?? "";
    return only.length >= 2 ? only.slice(0, 2).toUpperCase() : only.toUpperCase();
  }
  const first = parts[0] ?? "";
  const last = parts[parts.length - 1] ?? "";
  return ((first[0] ?? "?") + (last[0] ?? "")).toUpperCase();
}

// ---- 8.5 Tables ----------------------------------------------------------

/**
 * A data table. design.md 8.5: at least 48px rows, dividers rather than heavy
 * boxes, and numeric columns right-aligned. The wrapper scrolls horizontally on a
 * narrow screen rather than shrinking the type, because 8.5 forbids making text
 * smaller to fit more columns.
 */
export function Table({
  caption,
  children,
}: {
  caption?: string;
  children: ReactNode;
}) {
  return (
    <div className="table-wrap">
      <table className="table table--rows">
        {caption ? <caption>{caption}</caption> : null}
        {children}
      </table>
    </div>
  );
}

export function Th({
  children,
  numeric = false,
  scope = "col",
}: {
  children: ReactNode;
  numeric?: boolean;
  scope?: "col" | "row";
}) {
  return (
    <th scope={scope} className={numeric ? "table__num" : undefined}>
      {children}
    </th>
  );
}

export function Td({
  children,
  numeric = false,
  strong = false,
  meta = false,
}: {
  children: ReactNode;
  numeric?: boolean;
  strong?: boolean;
  meta?: boolean;
}) {
  const classes = [];
  if (numeric) classes.push("table__num");
  if (strong) classes.push("table__primary");
  if (meta) classes.push("table__meta");
  const className = classes.length > 0 ? classes.join(" ") : undefined;
  return <td className={className}>{children}</td>;
}

export function Tr({
  children,
  onClick,
}: {
  children: ReactNode;
  onClick?: () => void;
}) {
  return <tr onClick={onClick}>{children}</tr>;
}

/** design.md 7.5. */
export function DescriptionList({ children }: { children: ReactNode }) {
  return <dl className="description-list">{children}</dl>;
}

export function DescriptionTerm({ children }: { children: ReactNode }) {
  return <dt>{children}</dt>;
}

export function DescriptionValue({ children }: { children: ReactNode }) {
  return <dd>{children}</dd>;
}

export function CodeBlock({ children }: { children: string }) {
  return <pre className="code-block">{children}</pre>;
}

export function InlineCode({ children }: { children: string }) {
  return <code className="code-inline">{children}</code>;
}

/**
 * A distribution row: a name, a Progress, and the value.
 *
 * This is the composition design.md 10.4 specifies instead of a chart. Two
 * series are distinguished by the accent and the soft accent fill, always with
 * their labels, because two similar blues side by side are not distinguishable
 * on their own and there is no second hue available.
 */
export function DistributionRow({
  label,
  value,
  total,
  secondary,
}: {
  label: string;
  value: number;
  total: number;
  secondary?: boolean;
}) {
  return (
    <div className="weight-row">
      <span className="weight-row__name">{label}</span>
      <Progress label={label} value={value} total={total} {...(secondary ? { secondary } : {})} />
      <span className="weight-row__value">{value}</span>
    </div>
  );
}

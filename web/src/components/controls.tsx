import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Link } from "react-router-dom";

import { Icon, type IconName } from "../lib/icons";

/**
 * CodeCeremony components.
 *
 * design.md is the contract. The inventory in section 7 is closed: a component
 * that is not here does not get used, and a variant that is not here does not
 * get invented. Every visual value is a token from section 3 to 5, 9 and 11,
 * which is why there is no inline colour, spacing or radius anywhere in this
 * file. Every interactive component carries the eight states from section 7.6.
 */

// ---- 7.3 Controls ---------------------------------------------------------

/** The four variants in design.md 8.1, and the only four. */
export type ButtonVariant = "primary" | "secondary" | "tertiary" | "ghost";

/**
 * There is no destructive variant, because no destructive colour is approved.
 * A destructive action is Ghost, set apart from the primary action in its group,
 * and confirmed. design.md 10.3 records that decision and its consequences.
 */
function variantClass(variant: ButtonVariant): string {
  return `btn btn--${variant}`;
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  icon?: IconName;
  loading?: boolean;
}

export function Button({
  variant = "primary",
  icon,
  loading = false,
  children,
  className = "",
  disabled,
  ...rest
}: ButtonProps) {
  // design.md 7.6 loading state: the label stays in the DOM so the button keeps
  // its width and nothing around it shifts.
  return (
    <button
      type="submit"
      className={`${variantClass(variant)} ${className}`.trim()}
      disabled={disabled || loading}
      data-loading={loading ? "true" : undefined}
      aria-busy={loading || undefined}
      {...rest}
    >
      {!loading && icon ? <Icon name={icon} size={16} /> : null}
      {children}
    </button>
  );
}

export interface LinkButtonProps {
  to: string;
  variant?: ButtonVariant;
  icon?: IconName;
  children: ReactNode;
  className?: string;
  "aria-label"?: string;
}

/**
 * Navigation styled as a button.
 *
 * It is a link rather than a button that navigates, so it can be opened in a new
 * tab, copied, and read by assistive technology as a destination. design.md 12
 * warns against novel interaction patterns, and conflating these two is the
 * commonest one.
 */
export function LinkButton({
  to,
  variant = "secondary",
  icon,
  children,
  className = "",
  "aria-label": ariaLabel,
}: LinkButtonProps) {
  return (
    <Link to={to} className={`${variantClass(variant)} ${className}`.trim()} aria-label={ariaLabel}>
      {icon ? <Icon name={icon} size={16} /> : null}
      {children}
    </Link>
  );
}

export interface IconButtonProps {
  icon: IconName;
  /** Mandatory. design.md 13: every icon-only control has an accessible name. */
  label: string;
  onClick?: () => void;
  pressed?: boolean;
  href?: string;
  className?: string;
  type?: "button" | "submit";
}

/**
 * design.md 7.3. The label is rendered as visually-hidden text rather than an
 * aria-label, because hidden text reaches both a screen reader and the
 * browser's own accessibility tree, where an attribute on a bare glyph can be
 * lost.
 */
export function IconButton({
  icon,
  label,
  onClick,
  pressed,
  href,
  className = "",
  type = "button",
}: IconButtonProps) {
  const glyph = <Icon name={icon} size={20} />;
  const hidden = <span className="visually-hidden">{label}</span>;
  if (href) {
    return (
      <a href={href} className={`icon-btn ${className}`.trim()}>
        {glyph}
        {hidden}
      </a>
    );
  }
  return (
    <button
      type={type}
      className={`icon-btn ${className}`.trim()}
      onClick={onClick}
      aria-pressed={pressed}
    >
      {glyph}
      {hidden}
    </button>
  );
}

export interface FieldProps {
  id: string;
  label: string;
  hint?: string;
  error?: string;
  children: (props: { id: string; invalid: boolean; describedBy?: string }) => ReactNode;
}

/**
 * A labelled form control.
 *
 * design.md 13 requires a persistent label, so a placeholder is never the only
 * label. The error state carries an icon, text and border emphasis, never colour
 * alone, and the message is wired to the control with aria-describedby so it is
 * announced when focus lands.
 */
export function Field({ id, label, hint, error, children }: FieldProps) {
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [errorId, hintId].filter(Boolean).join(" ") || undefined;
  return (
    <div className="field" data-invalid={error ? "true" : undefined}>
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      {children({ id, invalid: Boolean(error), describedBy })}
      {error ? (
        // role="alert" so a message that appears after submission is announced
        // rather than only being visible. design.md 13: an error message has to
        // reach someone who cannot see the border change.
        <p className="field__error" id={errorId} role="alert">
          <Icon name="circle-alert" size={16} />
          {error}
        </p>
      ) : null}
      {hint && !error ? (
        <p className="field__hint" id={hintId}>
          {hint}
        </p>
      ) : null}
    </div>
  );
}

export interface SearchFieldProps {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}

/** design.md 7.3. A field with a leading glyph, not a new component. */
export function SearchField({ id, label, value, onChange, placeholder }: SearchFieldProps) {
  return (
    <Field id={id} label={label}>
      {({ id: fieldId }) => (
        <div className="search-field">
          <Icon name="search" size={20} />
          <input
            id={fieldId}
            className="input"
            type="search"
            value={value}
            placeholder={placeholder}
            onChange={(event) => onChange(event.target.value)}
          />
        </div>
      )}
    </Field>
  );
}

export interface CheckboxProps {
  id: string;
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}

export function Checkbox({ id, label, hint, checked, onChange, disabled }: CheckboxProps) {
  return (
    <label className="check" htmlFor={id}>
      <input
        id={id}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="check__text">
        {label}
        {hint ? <span className="check__hint">{hint}</span> : null}
      </span>
    </label>
  );
}

export interface SwitchProps {
  id: string;
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}

export function Switch({ id, label, checked, onChange, disabled }: SwitchProps) {
  return (
    <label className="switch" htmlFor={id}>
      <span>{label}</span>
      <input
        id={id}
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="switch__track" aria-hidden="true">
        <span className="switch__thumb" />
      </span>
    </label>
  );
}

export interface SelectProps {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
  hint?: string;
}

export function Select({ id, label, value, onChange, options, hint }: SelectProps) {
  return (
    <Field id={id} label={label} {...(hint ? { hint } : {})}>
      {({ id: fieldId, describedBy }) => (
        <select
          id={fieldId}
          className="select"
          value={value}
          aria-describedby={describedBy}
          onChange={(event) => onChange(event.target.value)}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}

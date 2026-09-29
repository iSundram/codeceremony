import { ICON_MARKUP } from "./icons.generated";

export type IconName = keyof typeof ICON_MARKUP;

/** The three sizes design.md 10.1 approves: 16 compact, 20 default, 24 large. */
export type IconSize = 16 | 20 | 24;

export interface IconProps {
  name: string;
  size?: IconSize;
  /**
   * Overrides the decorative default. The glyph is aria-hidden by default
   * because the accessible name belongs to the control that contains it, per
   * design.md 13. An icon that carries meaning on its own is rare, and when it
   * happens this is how it is stated.
   */
  label?: string;
}

/**
 * One approved Lucide glyph, inlined.
 *
 * design.md 10.1: a single family, outline, 1.75px stroke, nothing fetched at
 * runtime. The outer <svg> is written here rather than vendored so the size and
 * the accessibility attributes are set in exactly one place.
 *
 * An unknown name renders nothing rather than a broken glyph. That is silent by
 * design — a visible empty box would be worse — and the test suite asserts that
 * every name a component uses exists in the table, which turns it into a test
 * failure instead.
 */
export function Icon({ name, size = 20, label }: IconProps) {
  const markup = ICON_MARKUP[name];
  if (!markup) return null;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden={label ? undefined : true}
      role={label ? "img" : undefined}
      aria-label={label}
      focusable="false"
      className={`icon icon--${size}`}
      dangerouslySetInnerHTML={{ __html: markup }}
    />
  );
}

/** Every approved name, for the test that asserts the closed inventory holds. */
export const APPROVED_ICON_NAMES: readonly string[] = Object.keys(ICON_MARKUP).sort();

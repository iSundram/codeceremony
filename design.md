# CodeCeremony — Design System

> **Theme mode:** Light only for the first release.
>
> **Status:** Visual direction and component contract. Backend implementation is in progress; no frontend UI has been implemented yet.
>
> **Dependency:** Read `rules.md` before using this document.

## 1. Design contract

This file is the visual source of truth for CodeCeremony. Every future AI, designer, and developer must follow it.

### 1.1 Non-negotiable rules

1. Do not invent visual components. Use only the component inventory in this document.
2. If a required component is not listed, stop and add it to this document through an explicit design decision before implementing it.
3. Do not invent colors, gradients, shadows, fonts, radii, spacing values, icon styles, or animation curves.
4. The supplied palette is the complete color source. Accent roles are allowed and are assigned to supplied colors; they are not permission to introduce arbitrary new hues.
5. The first release is light theme only. Do not add a dark theme, theme switcher, or dark-mode tokens.
6. Do not copy a generic dashboard, Devpost clone, neon glassmorphism style, or unapproved visual reference.
7. Do not use raw color values in components. Use the semantic tokens defined here.
8. Do not create a new visual pattern for one component when an existing component or token can express the same idea.
9. Do not use a logo, illustration, icon, or font that has not been approved or supplied.
10. When this document and an implementation disagree, the implementation is wrong until the design is explicitly revised.

### 1.2 Meaning of “no extra color”

The palette has base colors and accent roles. An accent is a semantic use of an approved palette color, not an unapproved new color.

- Base colors provide the light canvas, text, neutral surfaces, and structural contrast.
- Accent colors provide emphasis, active states, focus, progress, and selected states.
- Alpha transparency of an approved color is allowed for hover, disabled, border, and shadow states.
- Alpha transparency must not introduce a new hue or be used to fake a new palette.
- If a visual idea requires a color not in the palette, it is not approved. Ask for a design decision rather than inventing one.

## 2. Visual identity

### 2.1 Concept

**Calm precision with ceremonial momentum.**

CodeCeremony should feel like a carefully prepared stage: structured, calm, trustworthy, and quietly confident. It should combine the clarity of a well-organized workspace with a distinct sense of arrival and achievement.

The visual language is:

- light blue atmosphere;
- deep ink typography;
- precise blue accents;
- soft warm-gray neutrals;
- controlled gradient ribbons;
- generous spacing;
- crisp geometry;
- quiet depth;
- purposeful motion.

### 2.2 Visual keywords

`clear`, `precise`, `layered`, `ceremonial`, `calm`, `confident`, `human`, `digital`, `structured`, `luminous`.

### 2.3 Anti-patterns

Do not use:

- dark theme colors in the first release;
- neon colors;
- rainbow gradients;
- glassmorphism used for decoration rather than for depth;
- blurry glow effects behind text; a canvas wash is permitted only where no text
  sits on it (§3.7);
- more than one dominant gradient at a time;
- arbitrary emoji as interface icons;
- mixed icon families;
- dense dashboard walls without hierarchy;
- decorative elements that compete with content;
- color-only status communication;
- new component patterns invented during implementation.

## 3. Color system

### 3.1 Approved palette

These are the only approved base colors for the first release.

| Token | Hex | Role |
|---|---|---|
| `ink` | `#33343B` | Primary text, strongest contrast, dark structural accent |
| `navy` | `#48547C` | Secondary text, primary action, focus ring, deep accent |
| `warm-gray` | `#AAA59F` | Neutral metadata, dividers, disabled states, warm counterpoint |
| `mist` | `#CFE7F8` | Main light canvas, pale surface, inverse text on dark actions |
| `accent-blue` | `#749DD0` | Primary accent, progress, selected state, emphasis |
| `accent-periwinkle` | `#92AAD1` | Secondary accent, borders, soft fills, gradient transition |

The accent colors are approved accent roles within the supplied palette. No additional accent hex values are introduced in this version.

**One documented exemption.** The brand assets in §10.2 are supplied artwork and
are used at their authored colours. The wordmark contains seven hues
(`#5B96E4`, `#4366AD`, `#96C7F7`, `#3A5793`, `#304057`, `#2D3C57`, and the
`#76A4DE` / `#96C7B6` / `#C3DFF7` / `#6078B6` / `#4C5E90` set in the icon), none
of which are in the palette above. Recolouring them would be redrawing the mark,
which §10.2 forbids, so the exemption is explicit rather than silent:

- the exemption covers **the mark only**, on any surface, at any size;
- it does **not** license a matching hue anywhere else. A button, chart, or
  border may not borrow `#5B96E4` because the logo uses it;
- an icon inside a button is governed by §10.1 and takes `color.text-*`, not the
  mark's blue.

The practical consequence is a small visible seam between the mark's saturated
blue and the interface's softer accent-blue. That is accepted rather than hidden:
the alternative is a logo that is not the logo.

### 3.2 Semantic tokens

| Semantic token | Value | Usage |
|---|---|---|
| `color.canvas` | `white` | Page background |
| `color.surface` | `white` | Primary card and panel surface |
| `color.surface-muted` | `accent-periwinkle` at ~8% over `white` | Quiet panels and secondary regions |
| `color.surface-raised` | `white` with approved border and shadow | Raised cards and focused containers |
| `color.surface-warm` | `warm-gray` at ~10% over `white` | Warm neutral panel; use sparingly |
| `color.text-primary` | `ink` | Headings, body copy, high-priority values |
| `color.text-secondary` | `navy` | Supporting copy, labels, navigation |
| `color.text-muted` | `#5A6A8C` | Nonessential metadata, never used for critical instructions |
| `color.text-disabled` | `warm-gray` | Disabled controls only |
| `color.text-on-dark` | `mist` | Text on `navy` or other deep approved surfaces |
| `color.border` | `navy` at 16% | Standard borders and dividers |
| `color.border-strong` | `navy` | Focus-adjacent or high-emphasis borders |
| `color.accent` | `accent-blue` | Active state, progress, selected state, primary emphasis |
| `color.accent-soft` | `accent-periwinkle` at 28% over `white` | Soft accent fill and decorative gradient partner |
| `color.focus` | `navy` | Keyboard focus ring |
| `color.overlay` | `navy` at 16% | Modal and drawer backdrop |
| `color.shadow` | `navy` at approved alpha levels | Depth and elevation |

**Surfaces are white; the palette is accents on top of them.** `mist` was
previously both the canvas and the surface, which put a blue cast on every card,
table and panel at once and left the brand colour nothing to be an accent
*against* — a theme where the base colour and the brand colour are the same
colour reads as one wash. White surfaces let `navy`, `accent-blue` and
`accent-periwinkle` do the small work they are for: a rail, an icon, a badge, a
hairline. All border and overlay values are alpha of an approved colour per
§1.2, never a new hue. §14, light only.

### 3.3 Accent usage

Accent colors are meant to be seen. Use them intentionally:

- `accent-blue` marks the current selection, active navigation, progress, or a primary visual emphasis.
- `accent-periwinkle` structures the interface through borders, soft fills, and gradient transitions.
- `navy` provides authority and focus; it is stronger than the lighter accents.
- `warm-gray` is a counterweight, not a primary text color on the light canvas.

Do not use an accent as a large field of text. Do not place small body text on `accent-blue` unless the text is large and bold and the contrast has been checked.

### 3.4 Contrast rules

The following combinations are approved for normal light-theme text:

| Foreground | Background | Contrast | Use |
|---|---|---:|---|
| `ink` | `mist` | 9.70:1 | Primary text |
| `navy` | `mist` | 5.80:1 | Secondary text and navigation |
| `mist` | `navy` | 5.80:1 | Text on deep primary actions |
| `ink` | `accent-periwinkle` | 5.25:1 | High-emphasis text on soft accent |
| `warm-gray` | `ink` | 5.07:1 | Reserved dark decorative surface only |

`warm-gray` on `mist` is approximately 1.91:1 and must not be used for normal text. `accent-blue` on `ink` is approximately 4.41:1; use it for large/bold text or non-text decoration only unless a separate accessibility decision is approved.

All text must meet WCAG AA contrast for its size and weight. If a design combination fails, change the token combination rather than adding a color.

**One consequence, measured rather than assumed.** Compositing `color.text-muted`
(`navy` at 72% over `mist`) gives `#6E7D9F`, which is **3.23:1**. That clears AA
for large text (3:1) and fails it for body text (4.5:1). The token is therefore
size-restricted: it is used at `heading.2` or above and for overlines, and
**never** for a form label, an instruction, a value a user must read, or anything
that carries meaning. Those use `color.text-secondary` at 5.80:1. The restriction
is a property of the specified token, not a licence to lighten or darken it, and
`design_test.go` asserts the 3.23:1 figure so a future blend change cannot pass
unnoticed.

### 3.5 Approved gradients

Only these gradient recipes are allowed:

| Token | Recipe | Use |
|---|---|---|
| `gradient.ribbon` | `linear-gradient(135deg, #CFE7F8 0%, #92AAD1 52%, #749DD0 100%)` | Hero accent, active brand moments, decorative side ribbon |
| `gradient.ink` | `linear-gradient(135deg, #48547C 0%, #749DD0 100%)` | Primary action emphasis or compact branded panel |
| `gradient.mist` | `linear-gradient(135deg, #CFE7F8 0%, #92AAD1 100%)` | Soft panel background and decorative wash |
| `gradient.warm` | `linear-gradient(135deg, #CFE7F8 0%, #AAA59F 100%)` | Rare warm-neutral counterpoint; never a dominant background |

Gradient rules:

- Use no more than one prominent gradient in a viewport.
- Gradients are decorative or supportive, not a substitute for hierarchy.
- Do not place small body text directly over `gradient.ink` unless the text token is `mist` and contrast is verified.
- Do not animate gradients by default.
- Do not add radial, conic, mesh, or multicolor gradients without a new design decision.
- Do not use a gradient as a full-page background behind dense content.

**`gradient.folded`** is added for the primary action. The three existing recipes
run 135°, which on a wide control reads as a flat band; the folded recipe runs
165° and moves through four stops, so a button or a brand panel has a direction
and a top edge rather than one colour with a hint of another.

| Token | Recipe | Use |
|---|---|---|
| `gradient.folded` | `linear-gradient(165deg, #48547C 0%, #33343B 52%, #48547C 100%)` | The primary action, and the brand panel on the sign-in surface |

**Why the ramp goes dark and then lightens again.** A primary button carries a
label, and the label has to be legible across the whole sweep, not at one end of
it. Measured across the approved palette, no label colour survives a light-to-dark
ramp: `mist` on `periwinkle` is **1.85:1** and on `accent-blue` is **2.20:1**,
while `ink` on `navy` is **1.67:1**. Any recipe that travels through the light
tones therefore has an unreadable patch in it, and the patch moves with the
gradient angle, so it cannot be fixed by choosing a different label.

So the ramp uses only `navy` and `ink`, which darkens through the middle and lifts
at the trailing edge: the colour of a folded card catching light on its far
side. `mist` on it measures **5.80:1 at its lightest and 9.70:1 at its
darkest**, so the label passes AA everywhere on the surface with no
angle-dependent patch. The fold highlight is carried by the inset edge in
`shadow.folded` rather than by a third colour, which is what keeps the ramp
honest.

A consequence worth stating: a filled primary action is a dark surface. `mist` on
`mist` is 1.00:1, so the label is `mist` and the button is never a light
surface carrying a dark label. Tinted buttons use `color.surface` with `ink`, per
§8.1.

A fifth recipe is approved for the page canvas only:

| Token | Recipe | Use |
|---|---|---|
| `canvas.wash` | `radial-gradient(at 12% -10%, rgba(146,170,209,0.30) 0px, transparent 55%), radial-gradient(at 92% 4%, rgba(201,220,234,0.55) 0px, transparent 50%)` | The `body` background, behind the opaque content panel |

This is the single place a radial gradient is allowed, and it is allowed because
of what it is *behind*. The canvas carries no text of its own: the content panel
is opaque and inset above it, so no glyph ever renders on a soft radial edge. The
rule is not "gradients on the canvas are fine" — it is "the canvas may carry
colour because nothing is read from it".

### 3.6 Borders and shadows

Borders:

- standard border: `1px solid` using `color.border`;
- strong border: `1px solid` using `color.border-strong`;
- focus ring: `2px solid` using `color.focus` with a `2px` offset;
- no arbitrary border widths or dashed borders except for an explicitly empty or unavailable state.

Approved elevation levels:

| Token | Value | Use |
|---|---|---|
| `shadow.0` | `0 1px 2px rgba(72, 84, 124, 0.08)` | Small controls and low-emphasis cards |
| `shadow.1` | `0 8px 24px rgba(72, 84, 124, 0.10)` | Cards and dropdown surfaces |
| `shadow.2` | `0 20px 50px rgba(72, 84, 124, 0.14)` | Drawers, modals, and major overlays |

Do not add glow shadows, black shadows, or arbitrary blur values.

**One exception, and it is the reason frosted surfaces read as glass rather than
as a pale rectangle.** A translucent surface has no edge of its own: without a
highlight along the top inner edge it looks like a hole where a panel should be.
So a frosted surface carries two shadows and no more:

| Token | Value | Use |
|---|---|---|
| `shadow.folded` | `inset 0 1px 0 rgba(255,255,255,0.55), 0 1px 2px rgba(72,84,124,0.06), 0 12px 32px rgba(72,84,124,0.10)` | Frosted panels: the top bar, the sidebar, the content panel, dropdowns |
| `shadow.lifted` | `inset 0 1px 0 rgba(255,255,255,0.65), 0 2px 4px rgba(72,84,124,0.08), 0 24px 56px rgba(72,84,124,0.16)` | Overlays above frosted surfaces: modals, drawers, menus |

The first component is the highlight and is what the exception is really about.
The two drop shadows are a tighter pair plus a wider one, which is the ordinary
way to say "this is above something" and is not a glow: there is no coloured or
blurred halo, and the alpha never exceeds 0.16. Two shadows remain the ceiling on
any surface; a third is a new design decision.

`shadow.brand` is added for a brand-coloured control that must read as pressed
on hover. It is navy-tinted rather than a coloured glow, and it is only ever
applied to a surface that already carries `gradient.folded`:

| Token | Value | Use |
|---|---|---|
| `shadow.brand` | `0 12px 28px -10px rgba(72,84,124,0.38)` | Primary action hover, brand panel |

### 3.7 Frosted surfaces

The document previously banned glassmorphism outright. That was protecting
something real — a blurred backdrop destroys text contrast, because the value
under the text changes with whatever scrolls past it — and the rule has been
rewritten to protect that thing directly instead of banning the technique.

A **frosted surface** is one that meets all four of these:

1. `backdrop-filter: blur(16px) saturate(140%)` over a surface tint of at least
   72% opacity. The blur is a background treatment, never a text treatment.
2. **No body text sits on the blurred area.** Text on a frosted surface is
   confined to the surface's own solid content region — a panel body, a menu
   column — or sits on an opaque child. A label floating directly on the blur is
   prohibited, because its measured contrast is a function of scroll position and
   therefore not measurable at all.
3. It carries `shadow.folded`, for the edge described in §3.6.
4. It is one of the approved surfaces: `top bar`, `sidebar`, `content panel`,
   `dropdown`, `auth panel`. Frost is a layout decision about hierarchy, so a card
   or an input is not a frosted surface and does not become one to look louder.

Frost is approved at `blur(16px)`. `blur(24px)` and above is reserved for the
auth panel, which is a single-purpose surface with no scrolling content behind
it; the more aggressive value is not a style upgrade, it is the cost of not being
able to put content behind that particular panel.

**Blur budget.** At most one frosted surface is stacked behind another, and no
frosted surface may contain another frosted surface. Nested frost is where this
technique turns to mush and where a performance cost that is defensible on one
panel becomes indefensible across a page.

## 4. Typography

### 4.1 Font stack

Use the following UI stack:

```text
Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif
```

Use the following monospace stack for code, identifiers, and technical metadata:

```text
"IBM Plex Mono", "SFMono-Regular", Consolas, "Liberation Mono", monospace
```

No additional display, script, decorative, or icon font may be introduced without a design decision.

### 4.2 Type scale

| Token | Size / line height | Weight | Use |
|---|---|---:|---|
| `display.1` | 48 / 56 | 800 | Major page or brand moment |
| `display.2` | 40 / 48 | 800 | Large headings |
| `heading.1` | 32 / 40 | 700 | Page title |
| `heading.2` | 24 / 32 | 700 | Section title |
| `heading.3` | 20 / 28 | 600 | Card or panel title |
| `body.large` | 18 / 28 | 400 | Introductory copy |
| `body` | 16 / 24 | 400 | Default reading text |
| `body.small` | 14 / 20 | 400 | Supporting text and controls |
| `label` | 13 / 18 | 600 | Form labels and compact headings |
| `overline` | 11 / 14 | 700 | Uppercase metadata; letter spacing `0.12em` |
| `code` | 13 / 20 | 400 | Code and identifiers |

Rules:

- Use only these sizes and weights.
- Use sentence case by default.
- Use uppercase only for overlines, compact metadata, and explicitly branded labels.
- **Navigation labels and section labels in the sidebar use `overline` treatment** —
  11px, weight 700, `letter-spacing: 0.12em`, uppercase. This is the one place
  uppercase is a structural choice rather than a metadata one, and it is listed
  here because it is what makes a long nav list scannable: a column of
  wide-tracked capitals reads as a set of categories, where a column of
  sentence-case words reads as a wall. It does not extend to a link inside body
  copy, a button, or a table cell.
- Keep body line length between approximately 60 and 75 characters.
- Use `text-wrap: balance` for short headings only.
- Do not use arbitrary letter spacing, line heights, or font sizes.
- Do not use all-caps paragraphs.

## 5. Spacing, layout, and grid

### 5.1 Spacing scale

Use only this spacing scale:

```text
4, 8, 12, 16, 20, 24, 32, 40, 48, 64, 80
```

The base unit is 4px. If a layout does not fit, change the composition before introducing a new value.

### 5.2 Application shell

Desktop application layout:

- left sidebar: 264px expanded;
- collapsed sidebar: 84px;
- top bar: 72px;
- content maximum width: 1440px;
- content grid: 12 columns;
- default grid gap: 24px;
- page vertical rhythm begins at 32px after the top bar.

The shell is light-theme only. It must not include a dark-mode control.

**The content panel is inset.** The shell paints `canvas.wash` on the body, and the
content sits on a frosted panel that is inset from the canvas by 4px on each
edge, carrying `radius.surface`. Horizontal padding moves inside that panel, to
32px at desktop, 24px at tablet, 16px at mobile.

This is the single largest change to the shell and it is structural rather than
decorative. Content on a bare canvas has no edge, so the page reads as an
undifferentiated field and the sidebar reads as a stripe on it; content on an
inset panel reads as a document sitting on a desk, and the frosted navigation
around it reads as chrome. It also means the scroll container is the panel rather
than the window, which is what keeps the top bar and the sidebar fixed without
`position: fixed` on four elements.

The panel's own scroll padding is 32px, rising to 40px at tablet and 48px at
wide. The extra breathing room at larger widths is deliberate: a content column
that runs edge to edge on a 1440px display has no measure and no hierarchy.

### 5.3 Breakpoints

| Name | Width | Layout behavior |
|---|---:|---|
| `mobile` | 0–767px | Single column; navigation becomes an overlay drawer |
| `tablet` | 768–1023px | Single or two-column content; compact sidebar allowed |
| `desktop` | 1024–1439px | Expanded or collapsed persistent sidebar |
| `wide` | 1440px and above | Centered maximum-width content with generous gutters |

Breakpoints are layout constraints, not permission to create new component variants.

### 5.4 Responsive rules

- Never introduce horizontal page scrolling at mobile widths.
- Preserve the same component semantics when layout changes.
- Move priority content before decorative content on small screens.
- Keep touch targets at least 44px by 44px.
- Do not hide essential labels; use an approved responsive pattern or visible disclosure.
- Do not create a separate mobile component for every desktop component.

## 6. App shell and sidebar specification

The sidebar is a core visual component and must follow this exact language.

### 6.1 Desktop sidebar

- Width: 264px expanded, 84px collapsed.
- Background: `color.surface` with a subtle `accent-periwinkle` alpha layer if needed.
- Right border: `1px solid color.border`.
- Logo/brand lockup sits at the top with consistent clear space.
- Navigation is grouped vertically with clear section labels.
- Active item uses `gradient.ribbon` or `color.accent-soft`; it is never hidden or represented by color alone.
- Hover uses a low-alpha `accent-periwinkle` background.
- Collapsed mode shows icons with accessible labels on focus and hover.
- The sidebar footer is reserved for approved utility content only; do not add promotional cards.
- The sidebar must not use a dark background in the light theme.

### 6.2 Mobile navigation

- Use the existing sidebar structure inside an overlay drawer.
- Drawer width: 280px maximum.
- Backdrop uses `color.overlay`.
- Opening and closing motion uses the approved motion tokens.
- Focus returns to the trigger when the drawer closes.
- Escape closes the drawer when safe.
- The same navigation item components are used; do not create a second mobile navigation design.

### 6.3 Top bar

- Height: 72px desktop, 56px mobile.
- Background: `color.surface` with `color.border` bottom border.
- Contains only the approved slots: navigation trigger, page context, utility actions, and status.
- Use `IconButton` for icon-only actions.
- Do not place a second navigation system or a new component in the top bar.

## 7. Allowed component inventory

No component outside this inventory may be used without first revising this document.

### 7.1 Foundations

- `Surface`
- `Stack`
- `Cluster`
- `Grid`
- `Text`
- `Icon`
- `Divider`
- `VisuallyHidden`

### 7.2 Navigation

- `AppShell`
- `Sidebar`
- `SidebarBrand`
- `SidebarNavGroup`
- `SidebarNavItem`
- `TopBar`
- `Breadcrumbs`
- `Tabs`
- `Pagination`

### 7.3 Controls

- `Button`
- `IconButton`
- `Input`
- `Textarea`
- `Select`
- `Checkbox`
- `Radio`
- `Switch`
- `FileInput`
- `SearchField`

### 7.4 Feedback and status

- `Badge`
- `Status`
- `Alert`
- `Toast`
- `Progress`
- `Tooltip`
- `Modal`
- `Drawer`
- `Popover`
- `Skeleton`
- `EmptyState`
- `ErrorState`

### 7.5 Data display

- `Card`
- `StatCard`
- `Table`
- `List`
- `DescriptionList`
- `Tag`
- `Avatar`
- `CodeBlock`

### 7.6 Component rules

Every interactive component must define and implement these states where applicable:

- default;
- hover;
- focus-visible;
- active or selected;
- disabled;
- loading;
- error;
- read-only, when applicable.

No component may introduce a new state pattern. If a state is needed and is not represented here, update the design contract first.

## 8. Component visual rules

### 8.1 Buttons

- Primary: `gradient.ink` background, `mist` text, `radius.control`, `shadow.0`.
- Secondary: `accent-periwinkle` background, `ink` text, `color.border` border.
- Tertiary: transparent background, `navy` text, `navy` border.
- Ghost: transparent background, `navy` text, visible on hover/focus only.
- Destructive styling is not approved in this palette. Do not invent a red destructive color; use an approved alert treatment and explicit confirmation pattern until a destructive palette is approved.
- Minimum height: 44px.
- Horizontal padding: 16px or 20px only.
- Label and icon gap: 8px.
- Do not use more than one filled button in a local action group unless the hierarchy is explicit.

### 8.2 Inputs

- Background: `color.surface`.
- Border: `1px solid color.border`.
- Radius: `radius.control` (12px).
- Minimum height: 44px.
- Focus: `color.focus` ring with 2px offset.
- Placeholder uses `color.text-muted` and must not replace a label.
- Error state uses an icon, text, and border emphasis; never color alone.
- Do not use floating labels unless the existing `Input` pattern explicitly supports them.

### 8.3 Cards and panels

- Radius: `radius.card` (16px).
- Standard border: `color.border`.
- Default elevation: `shadow.0` or no shadow.
- Raised or interactive cards may use `shadow.1` on hover.
- Use gradients only for a designated brand or emphasis surface, not every card.
- Card padding uses 20px, 24px, or 32px only.

### 8.4 Navigation items

- Height: 44px minimum.
- Radius: `radius.control` (12px).
- Icon and label gap: 12px.
- Active state is communicated with fill, weight, and an accessible state—not color alone.
- Labels must remain readable in collapsed mode through accessible naming.

### 8.5 Tables and lists

- Use row height of at least 48px.
- Use dividers instead of heavy boxes between rows.
- Use `color.text-secondary` for metadata.
- Keep numeric values right-aligned when comparing values.
- Do not use tiny text to fit more columns; use the approved responsive layout or overflow treatment.

### 8.6 Modal and drawer

- Modal maximum width: 640px.
- Drawer width: 280px, 360px, or 420px only.
- Overlay uses `color.overlay`.
- Surface uses `color.surface` and `shadow.2`.
- Title, close action, content, and actions follow a fixed vertical rhythm.
- Do not place a modal inside another modal.

### 8.7 Toasts and alerts

- Use `Toast` for transient, non-blocking feedback.
- Use `Alert` for persistent contextual information.
- Use `ErrorState` for a failed operation with no usable content.
- Use text and an approved icon; do not introduce red, green, orange, or other unreviewed status colors.

### 8.8 Progress and loading

- `Progress` uses `color.accent` on `color.surface-muted`.
- `Skeleton` uses `color.surface-muted` and `color.border`; it must not shimmer continuously by default.
- Loading states must not cause layout shifts.

## 9. Shape and depth

### 9.1 Radius scale

| Token | Value | Use |
|---|---:|---|
| `radius.small` | 8px | Small badges and compact marks |
| `radius.control` | 12px | Buttons and inputs |
| `radius.card` | 16px | Cards and panels |
| `radius.panel` | 24px | Large feature surfaces |
| `radius.surface` | 32px | The inset content panel, and the auth panel |
| `radius.feature` | 40px | The sign-in card and one other full-bleed feature surface per page |
| `radius.pill` | 999px | Pills and compact status indicators |

No other radius values are allowed.

The two additions above 24px exist for exactly two surfaces, and the restriction
is the point. A 24px corner on a 1200px panel reads as a rounded rectangle; a 32px
corner reads as a surface floating above another, because the radius is large
enough to describe the corner's curvature at that scale. `radius.feature` is
capped at one per page so it stays a signal rather than becoming the default.

### 9.2 Depth principles

- Prefer borders and spacing before shadows.
- Use one elevation level per surface.
- Increase elevation only for overlays or a meaningful interaction change.
- Do not use heavy black shadows, glows, or multiple stacked shadows.
- Do not use blur effects to create hierarchy.

## 10. Iconography and brand assets

### 10.1 Icons

**Approved family: Lucide** (MIT), outline set only.

- Use one approved icon family only.
- Default icon size: 20px.
- Compact icon size: 16px.
- Large icon size: 24px.
- Default stroke: 1.75px where the icon family supports strokes. Lucide is
  authored on a 24px grid at 2px; icons are rendered at `1.75px` so the set
  matches the weight of `ink` text at `body` size rather than reading heavier.
- Use outline icons for navigation and utility actions unless the approved family specifies otherwise.
- Do not mix filled, outline, duotone, and emoji icons in the same surface.
- Do not draw a new icon in CSS or substitute an icon when an approved one exists.
- Icons are inlined as SVG. No icon font, sprite sheet fetched at runtime, or
  external CDN: the portal must render with the network off.
- If a needed concept has no Lucide icon, the `Icon` component is used with the
  closest approved glyph and the accessible name carries the real meaning. A new
  glyph is not drawn.

### 10.2 Logo

**Approved assets.** Two files, from the brand owner's `codeceremony-logo`
repository, vendored at `backend/internal/httpapi/webassets/brand/`:

| Asset | viewBox | Use |
|---|---|---|
| `logo.svg` | `0 0 4096 1365` | Full lockup: icon plus the wordmark. Desktop sidebar, login, and any wide brand slot. |
| `icon.svg` | `0 0 2048 2048` | Mark only. Collapsed sidebar (84px), mobile drawer, favicon, and any square slot. |

They are **not** interchangeable. `logo.svg` in a square slot crops the wordmark;
`icon.svg` in a wide slot is an unlabelled mark where the wordmark belongs. The
component that owns the slot picks the correct file.

- Clear space is equal to the height of the smallest logo mark.
- The mark is used at its authored colours, exempt from §3.1 as documented there.
- Do not apply a gradient, glow, outline, or filter to the mark. The sidebar's
  own `gradient.ribbon` active state sits behind navigation items, never behind
  the brand lockup.
- Do not use a text substitute styled to imply it is the wordmark. The wordmark
  is artwork.

#### Two recorded deviations from the supplied files

Both are mechanical and neither alters the mark's geometry, and both are
recorded here rather than left for someone to discover in a diff:

1. **The opaque backdrop was removed.** As supplied, both files open with a
   white rectangle covering the whole viewBox. On a `mist` (`#CFE7F8`) sidebar
   that renders as a white box. The single path whose geometry is exactly the
   viewBox rectangle was deleted. `logo.svg` additionally contained two white
   paths forming the enclosed spaces inside two letters; with the backdrop gone
   they would render as white blobs, so they were removed as well. Every other
   path is byte-identical to the source, fill for fill.
2. **The brand owner should re-export both files with a transparent background**
   and no painted counters. The vendored copies are correct as rendered, but a
   first-party export is the right long-term artifact.

### 10.3 Status and destructive treatment

There is still no approved destructive or status palette, so **status is
communicated with text and an icon, never with a colour alone.** This is §8.7 and
§13 applied to every state the portal actually has.

| State | Treatment |
|---|---|
| Disqualified submission | `Badge` reading "Disqualified" with a `circle-slash` icon |
| Account suspended or pending deletion | `Status` reading the state with the matching Lucide icon |
| Audit chain verification failed | `Alert` with `alert-triangle`, titled "Chain verification failed" |
| Audit chain verified | `Alert` with `circle-check`, titled "Chain verified" |
| Pairwise fit is unbounded | `Alert` with `info`, explaining that the strengths are a truncated fit |
| Grant is expired | `Badge` reading "Expired" with `clock` |
| Explicit deny in force | `Badge` reading "Denied" with `shield-alert` |
| Destructive action offered | Tertiary or Ghost `Button`, explicit wording ("Revoke", "Withdraw"), and a confirmation `Modal` |

Two consequences worth stating, because they shape the UI:

- **"Destructive" is a wording and placement decision until a palette exists.** A
  destructive action is visually distinct by being Ghost, by sitting apart from
  the primary action in the group, and by requiring confirmation. It is not
  distinguished by a colour, because there is no approved one.
- **Every state above has a text label.** A user who cannot distinguish the icons
  reads the same information. Nothing in this system is conveyed by hue alone.

### 10.4 Data visualization

There is no separate chart component, because §7 has no room for one and adding
one would be inventing a visual pattern. Data displays are **composed from the
approved inventory**:

| Need | Composition |
|---|---|
| Judging progress | `Progress` per judge, in a `Card` |
| Counts and rates | `StatCard` with a `Text` value and a `Badge` for the delta |
| Distributions and separability | `Table` or `List` with `Progress` bars in a cell |
| Rankings | `Table` with a numeric column, right-aligned per §8.5 |
| Comparison outcomes | `Tag` plus `DescriptionList` |

Rules that follow from composing rather than charting:

- One prominent gradient per viewport, and **none** in a data display. A gradient
  in a chart would encode nothing and would compete with the content.
- `Progress` uses `color.accent` on `color.surface-muted`, per §8.8. Two series
  that must be distinguished use `color.accent` and `color.accent-soft` with
  distinct labels — never two similar blues alone.
- A trend is a number plus its direction in words ("12 of 18, up from 6"), not a
  sparkline. A sparkline would be a new visual pattern.
- Tables over 48px rows, right-aligned numerics, `color.text-secondary` for
  metadata, per §8.5. Never shrink type to fit more columns.

This is a deliberate limit. A scatter of normalized scores would be more
informative than a table of them, and it is not approved.

---

## 11. Motion

### 11.1 Motion tokens

| Token | Duration | Use |
|---|---:|---|
| `motion.instant` | 120ms | Press feedback and tiny state changes |
| `motion.fast` | 180ms | Hover, focus, and small component transitions |
| `motion.normal` | 240ms | Drawers, modals, and larger layout transitions |

Use these easing curves only:

```text
ease-out: cubic-bezier(0.16, 1, 0.3, 1)
standard: cubic-bezier(0.2, 0, 0, 1)
```

### 11.2 Motion rules

- Motion clarifies a change; it must not delay the user.
- Animate opacity, transform, and approved layout properties only.
- Do not animate gradients, blur, background image, or random decorative geometry.
- Do not add parallax, confetti, bouncing, continuous pulsing, or looping background animation.
- Respect `prefers-reduced-motion` and remove nonessential motion.
- Focus and state changes must remain visible when motion is reduced.

## 12. Interaction and UX principles

- Make the primary action obvious without relying on color alone.
- Keep navigation labels stable and predictable.
- Preserve user input when a request fails.
- Show progress for operations that take longer than 400ms.
- Use optimistic updates only when rollback behavior is defined.
- Provide clear empty, loading, and error states for every data surface.
- Use confirmation only for destructive or irreversible operations.
- Keep destructive actions visually distinct through wording and placement until an approved destructive color exists.
- Keep the reading order and keyboard order identical.
- Never hide essential information behind hover-only interactions.
- Use tooltips for supplementary information, never as the only label for an action.
- Do not create novel interaction patterns during implementation.

## 13. Accessibility requirements

- Target WCAG 2.2 AA for the light theme.
- Every interactive element has a visible `focus-visible` state.
- Focus rings use `color.focus`, 2px width, and 2px offset.
- Do not communicate status through color alone; pair it with text, icon, shape, or semantics.
- Use semantic HTML before ARIA.
- Every icon-only control has an accessible name.
- Form fields have persistent labels, not placeholder-only labeling.
- Error messages identify the field and the correction needed.
- Touch targets are at least 44px by 44px.
- Content remains usable at 200% zoom.
- Text and controls must meet the contrast combinations in this document.
- Decorative gradients must not reduce text readability.
- Respect reduced-motion preferences.
- Use a logical heading hierarchy and landmark structure.

## 14. Light-theme-only rule

Although the palette includes deep navy and ink, the first release remains a light theme.

- The page canvas is `mist`.
- Deep navy is used for controls, text, focus, and branded emphasis—not as a global dark background.
- No automatic dark-mode adaptation is permitted.
- No theme toggle, stored theme preference, or dark color token may be introduced.
- Future dark-theme work requires a separate approved design document and palette review.

## 15. AI implementation gate

Before generating any UI, an AI must verify all of the following:

**Decided — these gate whether implementation may begin.**

- [x] `rules.md` and `design.md` have been read. Re-read per surface; this is not
  a one-time check.
- [x] The implementation is light theme only.
- [x] Every used component is in the allowed inventory.
- [x] Every visual value comes from a token in this document.
- [x] No new hex, gradient, font, radius, shadow, spacing, or motion value is
  introduced.
- [x] No product feature or behavior was invented.
- [x] Any missing visual decision is reported instead of guessed.

**Verified — these are checked against the built output, not asserted here.**

- [ ] The sidebar follows the specified shell behavior (§5.2, §6).
- [ ] All interactive states are implemented (§7.6).
- [ ] Keyboard focus is visible (§13).
- [ ] Text contrast is acceptable (§3.4, §13).
- [ ] Mobile behavior uses the same component system (§5.4, §6.2).

The second list is empty on purpose. Items 4, 5 and 12 above were **no** until
this revision — the icon family, the logo, the status treatment and the
data-visualization limit were all undefined, and per §16 they had to be reported
rather than guessed. They are now decided in §10.1, §10.2, §10.3, §10.4 and §16.

The implementation items are deliberately unchecked, and each is backed by an
automated assertion in `backend/internal/httpapi/design_test.go` rather than by a
claim in this document. An unchecked box that turns out to be wrong is a smaller
problem than a checked box that was never verified.

If any answer is no, stop and report the gap. Do not silently create a new
component or visual system.

## 16. Deferred design decisions

The following are not defined yet and must not be invented during implementation:

- destructive/status **colors** beyond the approved palette — the treatment in
  §10.3 is decided and uses no new colour, but a palette would improve it;
- dark theme;
- product-specific page compositions;
- product-specific component variants;
- illustration assets;
- domain-specific content patterns.

These require explicit design decisions before use.

### Resolved since the first revision

| Was deferred | Now |
|---|---|
| final logo artwork and wordmark geometry | **Resolved.** §10.2 — `logo.svg` and `icon.svg` vendored, with two recorded deviations. |
| final icon library | **Resolved.** §10.1 — Lucide, outline, 1.75px, inlined. |
| destructive/status colors | **Partly resolved.** §10.3 decides the treatment as text plus icon. A status *palette* remains undecided. |
| data visualization styles | **Resolved as a limit.** §10.4 — no chart component; data displays compose from the approved inventory. |
| final font files and hosting strategy | **Resolved by default.** §4.1's stacks are used as declared, with the system fallbacks they already list. No font file is downloaded, self-hosted, or fetched at runtime, because §6.2 requires the portal to work with the network off and a webfont request is a network dependency. `Inter` and `IBM Plex Mono` are used when the operator has them installed locally, and the stack degrades to the system UI face otherwise. Hosting webfonts is a separate approved decision. |

## 17. Current handoff

- Product: CodeCeremony.
- Theme: light only.
- Base palette: `#33343B`, `#48547C`, `#AAA59F`, `#CFE7F8`, `#749DD0`, `#92AAD1`.
- Accent strategy: use `#749DD0` and `#92AAD1` as approved accent roles.
- Brand assets: `logo.svg` and `icon.svg` approved and vendored (§10.2).
- Icons: Lucide, outline, approved (§10.1).
- Status and destructive treatment: text plus icon, no new colour (§10.3).
- Data visualization: composition from the approved inventory, no chart
  component (§10.4).
- Product features: defined in `FEATURES.md`.
- Project code: backend foundation started; frontend UI not started.
- Next action: read `rules.md`, `FEATURES.md`, and this file before implementing any frontend surface.

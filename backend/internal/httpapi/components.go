package httpapi

import (
	"fmt"
	"html/template"
	"strings"
	"time"
)

// CodeCeremony UI components.
//
// design.md is the contract for everything in this file: the component set is
// closed (section 7), every visual value is a token (sections 3 to 5, 9 and 11),
// and an interactive component carries all eight states from section 7.6.
//
// These are Go template functions rather than a partials directory because each
// component is a small amount of markup around tokens, and having exactly one
// definition of each is what stops a page inventing a variant. The CSS in
// webassets/static carries the states; this file carries the markup.

// uiFuncs is the component set exposed to every template.
func (s *Server) uiFuncs() template.FuncMap {
	return template.FuncMap{
		// Components, design.md section 7.
		"icon":       renderIcon,
		"button":     renderButton,
		"linkButton": renderLinkButton,
		"iconButton": renderIconButton,
		"badge":      renderBadge,
		"statusPill": renderStatus,
		"alertBox":   renderAlert,
		"progress":   renderProgress,
		"statCard":   renderStatCard,
		"emptyState": renderEmptyState,
		"errorState": renderErrorState,
		"avatar":     renderAvatar,
		"tag":        renderTag,
		"yesNoBadge": renderYesNo,

		// Presentation helpers. Formatting is done here rather than in a template
		// so the rules are stated once and testable.
		"initials":  renderInitials,
		"count":     renderCount,
		"percent":   renderPercent,
		"pluralise": renderPlural,
		"shortDate": renderShortDate,
		"dateTime":  renderDateTime,
		"orDash":    renderOrDash,
		"truncate":  renderTruncate,
		"join":      strings.Join,
	}
}

// iconSizes are the three sizes design.md 10.1 approves. A request for any
// other size is clamped rather than honoured, so a stray value cannot introduce
// a dimension the contract does not have.
var iconSizes = map[int]bool{16: true, 20: true, 24: true}

// renderIcon emits an inline SVG from the approved Lucide set.
//
// The outer svg is written here rather than vendored so the size, the stroke
// width and the accessibility attributes are set in exactly one place. The glyph
// is aria-hidden because the accessible name belongs to the control that
// contains it, per design.md 13; an icon-only control must carry its own name.
//
// An unknown name renders nothing. That is a silent failure by design — a broken
// glyph would be worse — and design_test.go asserts that every name a template
// uses exists in the table, which turns it into a test failure instead.
func renderIcon(name string, size int) template.HTML {
	markup, ok := iconMarkup[name]
	if !ok {
		return ""
	}
	if !iconSizes[size] {
		size = 20
	}
	return template.HTML(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 24 24" `+
			`fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" `+
			`stroke-linejoin="round" aria-hidden="true" focusable="false" class="icon icon--%d">%s</svg>`,
		size, size, size, markup))
}

// buttonVariants are the four from design.md 8.1. There is no destructive
// variant, because no destructive colour is approved: a destructive action is
// Ghost, set apart from the primary action, and confirmed. Section 10.3 records
// that decision and its consequences.
var buttonVariants = map[string]bool{
	"primary": true, "secondary": true, "tertiary": true, "ghost": true,
}

func buttonVariant(variant string) string {
	if buttonVariants[variant] {
		return variant
	}
	return "tertiary"
}

// renderButton emits a submit Button. It is always a <button type="submit">:
// a control that navigates is a link, and renderLinkButton is used for that, so
// the two are never confused and a keyboard user is never handed a link that
// cannot be opened in a new tab.
func renderButton(label, variant, iconName string) template.HTML {
	if strings.TrimSpace(label) == "" {
		// design.md 13: a control with no accessible name is not a control. An
		// icon-only submit is almost always a mistake, so nothing is emitted.
		return ""
	}
	var b strings.Builder
	b.WriteString(`<button type="submit" class="btn btn--` + buttonVariant(variant) + `">`)
	if iconName != "" {
		b.WriteString(string(renderIcon(iconName, 16)))
	}
	b.WriteString(template.HTMLEscapeString(label) + `</button>`)
	return template.HTML(b.String())
}

// renderLinkButton emits an anchor styled as a button, for navigation.
func renderLinkButton(label, variant, iconName, href string) template.HTML {
	if strings.TrimSpace(label) == "" || strings.TrimSpace(href) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<a href="` + template.HTMLEscapeString(href) + `" class="btn btn--` + buttonVariant(variant) + `">`)
	if iconName != "" {
		b.WriteString(string(renderIcon(iconName, 16)))
	}
	b.WriteString(template.HTMLEscapeString(label) + `</a>`)
	return template.HTML(b.String())
}

// renderIconButton emits an IconButton for an icon-only control.
//
// design.md 13 requires an accessible name, so label is mandatory and is
// rendered as visually-hidden text rather than an aria-label: a hidden span is
// picked up by both a screen reader and the browser's own accessibility tree.
func renderIconButton(iconName, label, href string) template.HTML {
	if iconName == "" || strings.TrimSpace(label) == "" {
		return ""
	}
	icon := string(renderIcon(iconName, 20))
	hidden := `<span class="visually-hidden">` + template.HTMLEscapeString(label) + `</span>`
	if href != "" {
		return template.HTML(`<a href="` + template.HTMLEscapeString(href) +
			`" class="icon-btn">` + icon + hidden + `</a>`)
	}
	return template.HTML(`<button type="button" class="icon-btn">` + icon + hidden + `</button>`)
}

// badgeVariants are the three surfaces from design.md 8.3 and 7.4.
var badgeVariants = map[string]bool{"outline": true, "accent": true, "large": true}

// renderBadge emits a Badge. The label is the point: design.md 10.3 forbids
// communicating status with colour alone, so a badge always carries words and
// the icon is supplementary rather than load-bearing.
func renderBadge(label, iconName, variant string) template.HTML {
	if strings.TrimSpace(label) == "" {
		return ""
	}
	class := "badge"
	if badgeVariants[variant] {
		class += " badge--" + variant
	}
	var b strings.Builder
	b.WriteString(`<span class="` + class + `">`)
	if iconName != "" {
		b.WriteString(string(renderIcon(iconName, 16)))
	}
	b.WriteString(template.HTMLEscapeString(label) + `</span>`)
	return template.HTML(b.String())
}

// renderStatus emits a Status: a mark plus a word. Never a coloured word alone.
func renderStatus(label, iconName string, muted bool) template.HTML {
	if strings.TrimSpace(label) == "" {
		return ""
	}
	class := "status"
	if muted {
		class += " status--muted"
	}
	var b strings.Builder
	b.WriteString(`<span class="` + class + `">`)
	if iconName != "" {
		b.WriteString(string(renderIcon(iconName, 16)))
	} else {
		b.WriteString(`<span class="status__dot" aria-hidden="true"></span>`)
	}
	b.WriteString(template.HTMLEscapeString(label) + `</span>`)
	return template.HTML(b.String())
}

// alertIcons maps the approved Alert treatments to their glyph. There is no red
// or green anywhere in this map or in the stylesheet, per design.md 8.7 and
// 10.3: severity is carried by the icon and by the wording the caller supplies.
var alertIcons = map[string]string{
	"info":    "info",
	"success": "circle-check",
	"warning": "alert-triangle",
	"error":   "circle-alert",
	"neutral": "info",
}

func renderAlert(kind, title, body string) template.HTML {
	iconName, ok := alertIcons[kind]
	if !ok {
		iconName = alertIcons["info"]
	}
	var b strings.Builder
	b.WriteString(`<div class="alert`)
	if kind == "info" || kind == "neutral" {
		b.WriteString(` alert--muted`)
	}
	b.WriteString(`" role="note">`)
	b.WriteString(`<span class="alert__icon">` + string(renderIcon(iconName, 20)) + `</span>`)
	b.WriteString(`<div class="alert__body">`)
	if title != "" {
		b.WriteString(`<p class="alert__title">` + template.HTMLEscapeString(title) + `</p>`)
	}
	if body != "" {
		b.WriteString(`<p class="alert__text">` + template.HTMLEscapeString(body) + `</p>`)
	}
	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

// renderProgress emits a Progress bar per design.md 8.8: color.accent on
// color.surface-muted, with the value written out as text beside it so the bar
// is never the only carrier of the number, and with the ARIA values that give a
// screen reader the same information the pixels give a sighted user.
func renderProgress(label string, value, total int, secondary bool) template.HTML {
	if total <= 0 {
		total = 1
	}
	if value < 0 {
		value = 0
	}
	if value > total {
		value = total
	}
	class := "progress"
	if secondary {
		class += " progress--secondary"
	}
	var b strings.Builder
	b.WriteString(`<div class="` + class + `">`)
	if label != "" {
		b.WriteString(`<div class="progress__head"><span class="progress__label">` +
			template.HTMLEscapeString(label) + `</span>`)
		b.WriteString(fmt.Sprintf(`<span class="progress__value">%d of %d</span>`, value, total))
		b.WriteString(`</div>`)
	}
	b.WriteString(fmt.Sprintf(
		`<div class="progress__track" role="progressbar" aria-valuenow="%d" aria-valuemin="0" `+
			`aria-valuemax="%d" aria-label="%s"><div class="progress__fill" style="width:%d%%"></div></div>`,
		value, total, template.HTMLEscapeString(plainText(label)), value*100/total))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// renderStatCard emits a StatCard. The value is display type with tabular
// figures so a row of cards aligns on the digits.
func renderStatCard(label string, value int, meta, iconName string) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="stat-card">`)
	b.WriteString(`<p class="stat-card__label">` + template.HTMLEscapeString(label) + `</p>`)
	b.WriteString(`<p class="stat-card__value">` + fmt.Sprintf("%d", value) + `</p>`)
	if meta != "" || iconName != "" {
		b.WriteString(`<p class="stat-card__meta">`)
		if iconName != "" {
			b.WriteString(string(renderIcon(iconName, 16)))
		}
		if meta != "" {
			b.WriteString(template.HTMLEscapeString(meta))
		}
		b.WriteString(`</p>`)
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// renderEmptyState and renderErrorState cover design.md 12's requirement that
// every data surface has a clear empty, loading and error state.
func renderEmptyState(title, body, iconName string) template.HTML {
	if iconName == "" {
		iconName = "folder-kanban"
	}
	return template.HTML(`<div class="empty-state">` +
		`<span class="empty-state__icon">` + string(renderIcon(iconName, 24)) + `</span>` +
		`<p class="empty-state__title">` + template.HTMLEscapeString(title) + `</p>` +
		`<p class="empty-state__text">` + template.HTMLEscapeString(body) + `</p>` +
		`</div>`)
}

func renderErrorState(title, body string) template.HTML {
	return template.HTML(`<div class="error-state">` +
		`<span class="error-state__icon">` + string(renderIcon("circle-alert", 24)) + `</span>` +
		`<p class="error-state__title">` + template.HTMLEscapeString(title) + `</p>` +
		`<p class="error-state__text">` + template.HTMLEscapeString(body) + `</p>` +
		`</div>`)
}

// renderAvatar shows initials.
//
// No remote images. A gravatar would be a per-request third-party fetch keyed on
// a user's email address, and the one-command rule requires the portal to work
// with the network off. Deterministic per-user hues are also not used, because
// that would mean colours outside the approved palette.
func renderAvatar(name, size string) template.HTML {
	switch size {
	case "sm", "lg", "":
	default:
		size = ""
	}
	class := "avatar"
	if size != "" {
		class += " avatar--" + size
	}
	return template.HTML(`<span class="` + class + `" aria-hidden="true">` +
		template.HTMLEscapeString(initialsOf(name)) + `</span>`)
}

func renderTag(label string) template.HTML {
	if strings.TrimSpace(label) == "" {
		return ""
	}
	return template.HTML(`<span class="tag">` + template.HTMLEscapeString(label) + `</span>`)
}

// renderYesNo renders a boolean as a Badge with a word and an icon, because a
// bare "true" or a coloured dot is exactly the colour-only status communication
// design.md 10.3 rules out.
func renderYesNo(value bool, yesLabel, noLabel string) template.HTML {
	if yesLabel == "" {
		yesLabel = "Yes"
	}
	if noLabel == "" {
		noLabel = "No"
	}
	if value {
		return renderBadge(yesLabel, "check", "accent")
	}
	return renderBadge(noLabel, "minus", "outline")
}

// ---- presentation helpers ------------------------------------------------

func initialsOf(name string) string {
	fields := strings.Fields(name)
	switch len(fields) {
	case 0:
		return "?"
	case 1:
		if runes := []rune(fields[0]); len(runes) >= 2 {
			return strings.ToUpper(string(runes[:2]))
		} else if len(fields[0]) == 1 {
			return strings.ToUpper(fields[0])
		}
		return "?"
	default:
		first := []rune(fields[0])
		last := []rune(fields[len(fields)-1])
		if len(first) == 0 || len(last) == 0 {
			return "?"
		}
		return strings.ToUpper(string(first[0]) + string(last[0]))
	}
}

func renderInitials(name string) string { return initialsOf(name) }

// renderCount writes a bare number. The design contract specifies no locale or
// grouping format, so none is invented.
func renderCount(n int) string { return fmt.Sprintf("%d", n) }

func renderPercent(value, total int) string {
	if total <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", value*100/total)
}

func renderPlural(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func renderShortDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2 Jan 2006")
}

func renderDateTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2 Jan 2006, 15:04 MST")
}

// renderOrDash is the em-dash placeholder for a value that is genuinely absent,
// as distinct from a value that happens to be zero.
func renderOrDash(text string) string {
	if strings.TrimSpace(text) == "" {
		return "—"
	}
	return text
}

func renderTruncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func plainText(s string) string { return strings.Join(strings.Fields(s), " ") }

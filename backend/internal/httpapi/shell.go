package httpapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// The application shell's view model, and the app registry that fills it.
//
// The portal is one binary serving several applications side by side. Each app
// contributes a sidebar group and a route prefix; the shell decides which groups
// a given viewer may see. Nothing here decides what a viewer may *do* — that is
// the resolver's job, and a route that is absent from a sidebar is a
// convenience, not a control. Hiding an item is never authorization, which is
// what design.md and the event rules both require.

// navItem is one entry in the sidebar.
type navItem struct {
	Label   string
	Href    string
	Icon    string
	Current bool
}

// navGroup is a labelled section of the sidebar. design.md 6.1 requires the
// navigation to be grouped vertically with clear section labels.
type navGroup struct {
	Label string
	Items []navItem
}

type crumb struct {
	Label string
	Href  string
}

type topBarAction struct {
	Label   string
	Variant string
	Icon    string
	Href    string
}

// shellData is what the shell template renders.
type shellData struct {
	Title         string
	Breadcrumbs   []crumb
	Notice        string
	NoticeKind    string
	NavGroups     []navGroup
	TopBarActions []topBarAction
	NavToggle     bool

	SignedIn bool
	User     domain.User
	IsJudge  bool
	IsStaff  bool
	IsAdmin  bool

	// Nav is the app area currently being viewed, used to mark the active group.
	Nav string

	// EventSlug, EventName and Event describe the event a page is about. The
	// sidebar needs the name for the breadcrumb and the links need the slug.
	EventSlug string
	EventName string
	Event     domain.Event

	// Page carries the app's own view model. The shell does not inspect it, so
	// each app is free to shape its own content.
	Page any
}

// appDef declares one application in the shell.
type appDef struct {
	// Slug is the URL segment: /dashboard, /account, /judge, and so on.
	Slug string
	// Title is the sidebar group label and the default page title.
	Title string
	// Icon is the group's leading glyph, from the approved set.
	Icon string
	// Roles restricts which global roles see the group. Empty means every signed
	// in viewer. A visitor is handled separately by RequiresAuth.
	Roles []domain.Role
	// RequiresAuth hides the group from anonymous visitors.
	RequiresAuth bool
	// Order places the group in the sidebar. Lower is higher up.
	Order int
}

// apps is the registry, in sidebar order.
//
// This is the whole application map. Adding a surface is one entry here plus its
// route group, which is what keeps "separated by apps" from turning into a
// scattering of unrelated handlers.
var apps = []appDef{
	{Slug: "dashboard", Title: "Dashboard", Icon: "layout-dashboard", Order: 10},
	{
		Slug: "account", Title: "My account", Icon: "user",
		Order: 20, RequiresAuth: true,
	},
	{
		Slug: "events", Title: "Events", Icon: "calendar",
		Order: 30,
	},
	{
		Slug: "judge", Title: "Judging", Icon: "gavel",
		Order: 40, RequiresAuth: true, Roles: []domain.Role{domain.RoleJudge, domain.RoleOrganizer, domain.RoleAdmin},
	},
	{
		Slug: "organizer", Title: "Organizing", Icon: "clipboard-list",
		Order: 50, RequiresAuth: true, Roles: []domain.Role{domain.RoleOrganizer, domain.RoleAdmin},
	},
	{
		Slug: "admin", Title: "Administration", Icon: "shield",
		Order: 60, RequiresAuth: true, Roles: []domain.Role{domain.RoleAdmin},
	},
}

// maySee reports whether an app's sidebar group is shown to a viewer.
//
// This is a navigation convenience, not a control. Every route behind a group
// authorizes independently, so a viewer who reaches a page another way is
// refused by the resolver rather than by the sidebar having hidden the link.
func (a appDef) maySee(principal auth.Principal, signedIn bool) bool {
	if a.RequiresAuth && !signedIn {
		return false
	}
	if len(a.Roles) == 0 {
		return true
	}
	if !signedIn {
		return false
	}
	for _, role := range a.Roles {
		if principal.Role == role {
			return true
		}
	}
	return false
}

// linksFor returns a group's items for a viewer, with the current one marked.
func (a appDef) linksFor(path string, eventSlug string) []navItem {
	prefix := "/" + a.Slug
	items := []navItem{
		{Label: "Overview", Href: prefix, Icon: "layout-dashboard"},
	}
	switch a.Slug {
	case "events":
		if eventSlug != "" {
			items = append(items,
				navItem{Label: "Gallery", Href: prefix + "/" + eventSlug, Icon: "folder-kanban"},
				navItem{Label: "Projects", Href: prefix + "/" + eventSlug + "/projects", Icon: "list-checks"},
			)
		}
	case "account":
		items = append(items,
			navItem{Label: "Profile", Href: prefix + "/profile", Icon: "user"},
			navItem{Label: "Teams", Href: prefix + "/teams", Icon: "users"},
			navItem{Label: "Security", Href: prefix + "/security", Icon: "lock"},
		)
	case "judge":
		if eventSlug != "" {
			items = append(items,
				navItem{Label: "My assignments", Href: prefix + "/" + eventSlug, Icon: "clipboard-list"},
				navItem{Label: "Compare projects", Href: prefix + "/" + eventSlug + "/compare", Icon: "git-branch"},
			)
		}
	case "organizer":
		if eventSlug != "" {
			items = append(items,
				navItem{Label: "Progress", Href: prefix + "/" + eventSlug, Icon: "gauge"},
				navItem{Label: "Submissions", Href: prefix + "/" + eventSlug + "/submissions", Icon: "folder-kanban"},
				navItem{Label: "Panel", Href: prefix + "/" + eventSlug + "/panel", Icon: "users"},
				navItem{Label: "Results", Href: prefix + "/" + eventSlug + "/results", Icon: "trophy"},
				navItem{Label: "Audit", Href: prefix + "/" + eventSlug + "/audit", Icon: "shield"},
				navItem{Label: "Grants", Href: prefix + "/" + eventSlug + "/grants", Icon: "key"},
			)
		}
	case "admin":
		items = append(items,
			navItem{Label: "Accounts", Href: prefix + "/accounts", Icon: "users"},
			navItem{Label: "Platform audit", Href: prefix + "/audit", Icon: "shield"},
			navItem{Label: "Grants", Href: prefix + "/grants", Icon: "key"},
		)
	}
	// The current page is marked rather than coloured: design.md 8.4 requires
	// the active state to be carried by fill, weight and an accessible state.
	if path == prefix {
		items[0].Current = true
	}
	for i := range items {
		if i > 0 && path == items[i].Href {
			items[i].Current = true
		}
	}
	return items
}

// appForPath resolves the app a path belongs to.
func appForPath(path string) appDef {
	segment := strings.TrimPrefix(path, "/")
	if index := strings.IndexByte(segment, '/'); index >= 0 {
		segment = segment[:index]
	}
	for _, app := range apps {
		if app.Slug == segment {
			return app
		}
	}
	return appDef{}
}

// shellFor builds the shell view model for a request.
//
// eventSlug is the event the page is about, which is what lets each group offer
// its event-scoped links. It is derived from the path or an explicit argument,
// never guessed.
func (s *Server) shellFor(r *http.Request, title, nav string, eventSlug string) shellData {
	data := shellData{Title: title, Nav: nav, NavToggle: true}

	if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
		if user, err := s.store.UserByID(principal.UserID); err == nil {
			data.SignedIn = true
			data.User = user
			data.IsJudge = user.Role == domain.RoleJudge
			data.IsStaff = user.Role.IsStaff()
			data.IsAdmin = user.Role == domain.RoleAdmin
		}
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}

	for _, app := range apps {
		if !app.maySee(auth.Principal{Role: roleOf(data)}, data.SignedIn) {
			continue
		}
		data.NavGroups = append(data.NavGroups, navGroup{
			Label: app.Title,
			Items: app.linksFor(path, eventSlug),
		})
	}

	// A signed-in viewer's own sign-out lives in the top bar rather than the
	// sidebar, because it is a session action rather than a destination.
	if data.SignedIn {
		data.TopBarActions = append(data.TopBarActions, topBarAction{
			Label: "Account", Variant: "tertiary", Icon: "user", Href: "/account",
		})
	} else {
		data.TopBarActions = append(data.TopBarActions, topBarAction{
			Label: "Sign in", Variant: "primary", Icon: "log-in", Href: "/login",
		})
	}
	return data
}

func roleOf(data shellData) domain.Role { return data.User.Role }

// eventSlugFor resolves the event a path is about, so the sidebar can offer
// event-scoped links without the shell guessing.
func (s *Server) eventSlugFor(r *http.Request) string {
	if slug := r.PathValue("slug"); slug != "" {
		return slug
	}
	if slug := r.PathValue("eventSlug"); slug != "" {
		return slug
	}
	return ""
}

// crumbsFor builds a breadcrumb trail from a path, resolving slugs to names so
// the trail reads as words rather than identifiers.
func (s *Server) crumbsFor(r *http.Request, trail ...crumb) []crumb {
	out := append([]crumb{{Label: "Home", Href: "/dashboard"}}, trail...)
	// Any trailing element with a resolved event name reads better than a slug.
	slug := s.eventSlugFor(r)
	if slug != "" {
		if event, err := s.store.EventBySlug(slug); err == nil {
			for i := range out {
				if out[i].Href != "" && strings.HasSuffix(out[i].Href, "/"+slug) {
					out[i].Label = event.Name
				}
			}
		}
	}
	return out
}

// sortEventsByName is used wherever a list of events is shown, so ordering is
// deterministic rather than dependent on map iteration.
func sortEventsByName(events []domain.Event) {
	sort.Slice(events, func(i, j int) bool { return events[i].Name < events[j].Name })
}

// eventOrFirst resolves an event for a page, preferring the path slug and
// falling back to the newest event so a page with no event context still
// renders. The fallback is the store's first event, not a guess at which event
// "matters".
func (s *Server) eventOrFirst(r *http.Request) (domain.Event, bool) {
	if slug := s.eventSlugFor(r); slug != "" {
		if event, err := s.store.EventBySlug(slug); err == nil {
			return event, true
		}
	}
	if id := r.URL.Query().Get("event_id"); id != "" {
		if event, err := s.store.EventByID(id); err == nil {
			return event, true
		}
	}
	events := s.store.ListEvents()
	if len(events) == 0 {
		return domain.Event{}, false
	}
	sortEventsByName(events)
	return events[0], true
}

// eventsFor renders a list of events for a nav or picker.
func (s *Server) eventsFor() []domain.Event { return s.store.ListEvents() }

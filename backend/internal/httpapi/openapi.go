package httpapi

import (
	"net/http"
	"sort"
	"strings"
)

// RouteInfo describes one registered route. The catalog is generated from the
// live mux, so it can never drift from the implementation.
type RouteInfo struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
	Auth    string `json:"auth"`
	Public  bool   `json:"public"`
	Group   string `json:"group"`
}

var routeAuthRules = []struct {
	prefix   string
	auth     string
	publicOK bool
}{
	{"/healthz", "none", true},
	{"/readyz", "none", true},
	{"/v1/openapi.json", "none", true},
	{"/v1/endpoints", "none", true},
	{"/v1/permissions", "none", true},
	{"/v1/directory", "none", true},
	{"/v1/events", "none", true},
	{"/v1/judging/rubric", "none", true},
	{"/v1/unsubscribe", "none", true},
	{"/v1/auth", "none", true},
	{"/v1/activity", "session", false},
	{"/v1/admin", "session:admin", false},
	{"/v1/organizer", "session:organizer", false},
	{"/v1/account", "session", false},
	{"/v1/notifications", "session", false},
	{"/v1/email", "session", false},
	{"/v1/judge", "session:judge", false},
	{"/v1/teams", "session", false},
	{"/v1/submissions", "session", false},
	{"/v1/profiles", "session", false},
	{"/v1/discover", "session", false},
	{"/v1/invites", "session", false},
	{"/v1/vote", "session", false},
	{"/v1/comments", "session", false},
	{"/v1/users", "session", false},
	{"/v1/me", "session", false},
}

func classifyRoute(method, pattern string) (string, bool, string) {
	path := pattern
	if strings.HasPrefix(path, "/v1/events/") || strings.HasPrefix(path, "/v1/directory") {
		if method == http.MethodGet {
			return "none", true, groupFor(path)
		}
		return "session:organizer_or_owner", false, groupFor(path)
	}
	for _, rule := range routeAuthRules {
		if strings.HasPrefix(path, rule.prefix) {
			return rule.auth, rule.publicOK && method == http.MethodGet, groupFor(path)
		}
	}
	return "unknown", false, groupFor(path)
}

var routeGroupOverrides = []struct {
	prefix string
	group  string
}{
	{"/v1/events/{slug}/comments", "community"},
	{"/v1/events/{slug}/projects/{projectID}/comments", "community"},
	{"/v1/comments", "community"},
	{"/v1/events/{slug}/vote", "community"},
	{"/v1/events/{slug}/vote-campaigns", "community"},
	{"/v1/vote", "community"},
	{"/v1/events/{slug}/webhooks", "community"},
	{"/v1/events/{slug}/leaderboard", "events"},
	{"/v1/judging", "judge"},
	{"/v1/profile", "profiles"},
	{"/v1/unsubscribe", "mail"},
	{"/v1/permissions", "activity"},
	{"/v1/activity", "activity"},
	{"/v1/directory", "events"},
	{"/v1/openapi.json", "system"},
	{"/v1/endpoints", "system"},
}

func groupFor(path string) string {
	for _, override := range routeGroupOverrides {
		if strings.HasPrefix(path, override.prefix) {
			return override.group
		}
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 {
		return "root"
	}
	if parts[0] != "v1" {
		return "system"
	}
	if len(parts) < 2 {
		return "system"
	}
	return parts[1]
}

func (s *Server) routeCatalog(w http.ResponseWriter, r *http.Request) {
	pattern := s.routePatterns(r)
	routes := make([]RouteInfo, 0, len(pattern))
	groups := map[string]int{}
	authCounts := map[string]int{}
	for _, entry := range pattern {
		auth, public, group := classifyRoute(entry.Method, entry.Pattern)
		routes = append(routes, RouteInfo{Method: entry.Method, Pattern: entry.Pattern, Auth: auth, Public: public, Group: group})
		groups[group]++
		authCounts[auth]++
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Group == routes[j].Group {
			if routes[i].Pattern == routes[j].Pattern {
				return routes[i].Method < routes[j].Method
			}
			return routes[i].Pattern < routes[j].Pattern
		}
		return routes[i].Group < routes[j].Group
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"data":         routes,
		"count":        len(routes),
		"by_group":     groups,
		"by_auth":      authCounts,
		"public_count": countPublic(routes),
		"note":         "generated from the live router, so it always matches the running service",
	})
}

func countPublic(routes []RouteInfo) int {
	count := 0
	for _, route := range routes {
		if route.Public {
			count++
		}
	}
	return count
}

func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "CodeCeremony API",
			"version":     "0.4.0",
			"description": "Hackathon hosting, submissions, judging, community, and mail API. Sessions are opaque bearer tokens issued by POST /v1/auth/login and sent as `Authorization: Bearer <token>` or the `session` cookie.",
			"license":     map[string]string{"name": "MIT"},
		},
		"servers": []map[string]string{{"url": "/"}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearer": map[string]string{"type": "http", "scheme": "bearer"},
				"cookie": map[string]string{"type": "apiKey", "in": "cookie", "name": "session"},
			},
			"schemas": openAPISchemas(),
		},
		"x-route-groups": []map[string]any{
			{"group": "auth", "summary": "local login, logout, and available auth methods"},
			{"group": "account", "summary": "first-party My Account: profile, password, sessions, export, deletion"},
			{"group": "admin", "summary": "platform administration: users, roles, sessions, audit"},
			{"group": "events", "summary": "hackathon directory, configuration, tracks, prizes, hosts, milestones, questions, staff, roster, leaderboard"},
			{"group": "teams", "summary": "team formation, roles, invitations"},
			{"group": "submissions", "summary": "submission lifecycle, eligibility, duplicates, version history"},
			{"group": "organizer", "summary": "assignment builder, rubrics, results publication, announcements, reminders, export, import, webhooks, mail"},
			{"group": "judge", "summary": "assignments, reviews, own scores"},
			{"group": "community", "summary": "comments, moderation, reports, voting campaigns"},
			{"group": "activity", "summary": "activity log and permission matrix"},
			{"group": "mail", "summary": "preferences, verification, unsubscribe, outbox"},
		},
		"paths": openAPIPaths(s.routePatterns(r)),
	})
}

func openAPIPaths(patterns []RouteEntry) map[string]any {
	paths := map[string]any{}
	for _, entry := range patterns {
		auth, public, group := classifyRoute(entry.Method, entry.Pattern)
		path, parameters := openAPIPath(entry.Pattern)
		if path == "" {
			continue
		}
		item, ok := paths[path].(map[string]any)
		if !ok {
			item = map[string]any{}
			paths[path] = item
		}
		item[strings.ToLower(entry.Method)] = map[string]any{
			"operationId": operationID(entry.Method, entry.Pattern),
			"summary":     summaryFor(group, entry.Method, entry.Pattern),
			"tags":        []string{group},
			"x-auth":      auth,
			"x-public":    public,
			"parameters":  parameters,
			"responses": map[string]any{
				"200": map[string]any{"description": "success"},
				"400": map[string]any{"description": "invalid request"},
				"401": map[string]any{"description": "authentication required"},
				"403": map[string]any{"description": "forbidden"},
				"404": map[string]any{"description": "not found"},
				"422": map[string]any{"description": "validation error"},
			},
		}
	}
	return paths
}

func openAPIPath(pattern string) (string, []any) {
	if !strings.HasPrefix(pattern, "/") {
		return "", nil
	}
	segments := strings.Split(pattern, "/")
	parameters := make([]any, 0)
	for index, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := strings.Trim(segment, "{}")
			parameters = append(parameters, map[string]any{
				"name":     name,
				"in":       "path",
				"required": true,
				"schema":   map[string]any{"type": "string"},
			})
			segments[index] = "{" + name + "}"
		}
	}
	return strings.Join(segments, "/"), parameters
}

func operationID(method, pattern string) string {
	cleaned := pattern
	cleaned = strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_").Replace(cleaned)
	return strings.ToLower(method) + cleaned
}

func summaryFor(group, method, pattern string) string {
	verb := map[string]string{
		http.MethodGet:    "read",
		http.MethodPost:   "create",
		http.MethodPatch:  "update",
		http.MethodPut:    "replace",
		http.MethodDelete: "remove",
	}[method]
	if verb == "" {
		verb = strings.ToLower(method)
	}
	return verb + " " + group + " resource"
}

func openAPISchemas() map[string]any {
	return map[string]any{
		"Envelope": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data":  map[string]any{"description": "the resource or list of resources"},
				"count": map[string]any{"type": "integer"},
				"meta":  map[string]any{"type": "object"},
			},
		},
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":    map[string]any{"type": "string"},
						"message": map[string]any{"type": "string"},
					},
				},
			},
		},
		"Event": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                  map[string]any{"type": "string"},
				"slug":                map[string]any{"type": "string"},
				"name":                map[string]any{"type": "string"},
				"summary":             map[string]any{"type": "string"},
				"state":               map[string]any{"type": "string", "enum": []string{"draft", "registration_open", "submissions_open", "submissions_closed", "judging", "results_published", "archived"}},
				"judging_mode":        map[string]any{"type": "string", "enum": []string{"automatic", "manual"}},
				"registration_open":   map[string]any{"type": "boolean"},
				"submissions_open":    map[string]any{"type": "boolean"},
				"submissions_close":   map[string]any{"type": "string", "format": "date-time"},
				"min_team_size":       map[string]any{"type": "integer"},
				"max_team_size":       map[string]any{"type": "integer"},
				"allow_global_teams":  map[string]any{"type": "boolean"},
				"reviews_per_project": map[string]any{"type": "integer"},
				"results_published":   map[string]any{"type": "boolean"},
			},
		},
		"Submission": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":             map[string]any{"type": "string"},
				"event_id":       map[string]any{"type": "string"},
				"team_id":        map[string]any{"type": "string"},
				"track_id":       map[string]any{"type": "string"},
				"title":          map[string]any{"type": "string"},
				"summary":        map[string]any{"type": "string"},
				"story":          map[string]any{"type": "string"},
				"repo_url":       map[string]any{"type": "string", "format": "uri"},
				"live_url":       map[string]any{"type": "string", "format": "uri"},
				"status":         map[string]any{"type": "string", "enum": []string{"draft", "submitted", "needs_changes", "withdrawn", "locked", "disqualified"}},
				"eligibility":    map[string]any{"type": "string", "enum": []string{"pending", "eligible", "ineligible"}},
				"custom_answers": map[string]any{"type": "object"},
				"version":        map[string]any{"type": "integer"},
			},
		},
		"Review": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":             map[string]any{"type": "string"},
				"judge_id":       map[string]any{"type": "string"},
				"project_id":     map[string]any{"type": "string"},
				"criteria":       map[string]any{"type": "object"},
				"normalized":     map[string]any{"type": "object"},
				"rubric_id":      map[string]any{"type": "string"},
				"rubric_version": map[string]any{"type": "integer"},
				"submitted":      map[string]any{"type": "boolean"},
			},
		},
		"Assignment": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"judge_id":   map[string]any{"type": "string"},
				"project_id": map[string]any{"type": "string"},
				"strategy":   map[string]any{"type": "string", "enum": []string{"manual", "balanced", "batch"}},
				"revoked_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"ActivityEntry": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"category":   map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string"},
				"summary":    map[string]any{"type": "string"},
				"visibility": map[string]any{"type": "string", "enum": []string{"public", "participants", "judges", "organizers"}},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"MailPreferences": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_id":           map[string]any{"type": "string"},
				"transactional":     map[string]any{"type": "boolean"},
				"account_security":  map[string]any{"type": "boolean"},
				"account_lifecycle": map[string]any{"type": "boolean"},
				"team_activity":     map[string]any{"type": "boolean"},
				"event_activity":    map[string]any{"type": "boolean"},
				"judging":           map[string]any{"type": "boolean"},
				"results":           map[string]any{"type": "boolean"},
				"marketing":         map[string]any{"type": "boolean"},
				"weekly_digest":     map[string]any{"type": "boolean"},
				"unsubscribed_all":  map[string]any{"type": "boolean"},
			},
		},
	}
}

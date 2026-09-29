package httpapi

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/judging"
	"github.com/iSundram/codeceremony/backend/internal/ratelimit"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// Rate-limit budgets, referenced by the form handlers as well as the API routes so
// there is one policy per abuse class rather than one per call site.
var (
	ratelimitLogin   = ratelimit.Login
	ratelimitAccount = ratelimit.Account
	ratelimitBallot  = ratelimit.Ballot
	ratelimitComment = ratelimit.Comment
	ratelimitWrite   = ratelimit.Write
)

//go:embed webassets
var webAssets embed.FS

// The HTML frontend is server-rendered on purpose. A separate single-page app
// would need a Node toolchain, a package install and a build step inside the
// container, and the event's one-command rule is that a laptop with the network
// off comes up with a working portal. Templates are compiled into the binary
// and the stylesheet is embedded, so there is nothing to fetch and nothing to
// build. The JSON API is not a fallback for the pages: it is the same data,
// and the pages read it through the same store.
var webPages = map[string]string{
	"home":      "home.html",
	"gallery":   "gallery.html",
	"project":   "project.html",
	"login":     "login.html",
	"judge":     "judge.html",
	"organizer": "organizer.html",
	"results":   "results.html",
	"account":   "account.html",
	"error":     "error.html",
}

// renderPage compiles a page with the shared layout the first time it is
// needed and caches it. Each page defines "content", so they cannot all be
// parsed into one template set; the layout is shared and the content block is
// per page.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, page string, data shellData) {
	tmpl, err := s.pageTemplate(page)
	if err != nil {
		s.logger.Error("could not render a page", "page", page, "error", err)
		writeError(w, http.StatusInternalServerError, "render_error", "the page could not be rendered")
		return
	}
	var buffer bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buffer, "shell", data); err != nil {
		s.logger.Error("could not execute a page template", "page", page, "error", err)
		writeError(w, http.StatusInternalServerError, "render_error", "the page could not be rendered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The portal is same-origin, so a restrictive policy costs nothing and rules
	// out the obvious injection vectors for a server-rendered app.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; style-src 'self'; img-src 'self' data:; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	_, _ = w.Write(buffer.Bytes())
}

// pageData builds the shell view model, resolving the caller's identity from
// the same session logic the API uses so the pages and the JSON routes cannot
// disagree about who is signed in.
func (s *Server) pageData(r *http.Request, title, nav string) shellData {
	data := s.shellFor(r, title, nav, s.eventSlugFor(r))
	if event, err := s.store.EventBySlug(seedSlug); err == nil {
		if data.EventSlug == "" {
			data.EventSlug = event.Slug
		}
		data.EventName = event.Name
	}
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
		if user, err := s.store.UserByID(principal.UserID); err == nil {
			data.Event = user2event(user)
		}
	}
	if data.Event.ID == "" {
		if event, err := s.store.EventBySlug(data.EventSlug); err == nil {
			data.Event = event
		}
	}
	return data
}

// user2event is a placeholder kept while the account app is migrated; the shell
// only needs the signed-in user's role, which shellFor already resolved.
func user2event(user domain.User) domain.Event { return domain.Event{} }

// seedSlug is the fixture event, used only to title the masthead. The gallery
// links are always derived from the event a page is actually about.
const seedSlug = "sample-hack-2026"

func (s *Server) pageTemplate(page string) (*template.Template, error) {
	file, ok := webPages[page]
	if !ok {
		return nil, fmt.Errorf("unknown page %q", page)
	}
	s.pagesMu.Lock()
	defer s.pagesMu.Unlock()
	if tmpl, ok := s.pages[page]; ok {
		return tmpl, nil
	}
	funcs := template.FuncMap{"join": strings.Join}
	for name, fn := range s.uiFuncs() {
		funcs[name] = fn
	}
	tmpl, err := template.New("codeceremony").Funcs(funcs).ParseFS(
		webAssets,
		"webassets/templates/shell.html",
		"webassets/templates/"+file,
	)
	if err != nil {
		return nil, err
	}
	s.pages[page] = tmpl
	return tmpl, nil
}

// ---- pages ----------------------------------------------------------------

func (s *Server) webHome(w http.ResponseWriter, r *http.Request) {
	data := s.pageData(r, "Events", "")
	events := s.store.ListEvents()
	sort.Slice(events, func(i, j int) bool { return events[i].Name < events[j].Name })
	type eventCard struct {
		Slug, Name, State, Closes string
		ProjectCount, JudgeCount  int
	}
	cards := make([]eventCard, 0, len(events))
	for _, event := range events {
		roster := s.store.JudgeRoster(event.ID)
		cards = append(cards, eventCard{
			Slug: event.Slug, Name: event.Name, State: string(event.State),
			Closes:       event.SubmissionsClose.Format("2 Jan 2006"),
			ProjectCount: len(s.store.ListSubmissions(event.ID, "", "")),
			JudgeCount:   len(roster),
		})
	}
	data.Page = struct {
		Heading string
		Lede    string
		Events  []eventCard
	}{
		Heading: "Events",
		Lede:    "An open source submission and judging platform. Every project below is seeded from the shared DOGFOOD fixtures.",
		Events:  cards,
	}
	s.renderPage(w, r, "home", data)
}

func (s *Server) webGallery(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		s.webNotFound(w, r, "That event does not exist.")
		return
	}
	query := r.URL.Query().Get("q")
	trackID := r.URL.Query().Get("track")

	tracks := s.store.ListTracks(event.ID)
	projects := s.store.ListSubmissions(event.ID, query, trackID)

	type card struct {
		ID, Title, Summary, TeamName, TrackName string
	}
	cards := make([]card, 0, len(projects))
	counts := make(map[string]int, len(tracks))
	for _, track := range tracks {
		counts[track.ID] = 0
	}
	for _, project := range projects {
		name := ""
		if track, err := s.store.TrackByID(project.TrackID); err == nil {
			name = track.Name
		}
		counts[project.TrackID]++
		team := ""
		if record, err := s.store.TeamByID(project.TeamID); err == nil {
			team = record.Name
		}
		cards = append(cards, card{ID: project.ID, Title: project.Title, Summary: project.Summary, TeamName: team, TrackName: name})
	}
	type trackRow struct {
		ID, Name string
		Count    int
	}
	rows := make([]trackRow, 0, len(tracks))
	for _, track := range tracks {
		rows = append(rows, trackRow{ID: track.ID, Name: track.Name, Count: counts[track.ID]})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	published := event.ResultsPublished
	data := s.pageData(r, event.Name, "event")
	data.Event = event
	data.Page = struct {
		Event            domain.Event
		Projects         []card
		Tracks           []trackRow
		Query            string
		TrackID          string
		ProjectCount     int
		JudgeCount       int
		ResultsPublished bool
	}{
		Event: event, Projects: cards, Tracks: rows,
		Query: query, TrackID: trackID,
		ProjectCount:     len(s.store.ListSubmissions(event.ID, "", "")),
		JudgeCount:       len(s.store.JudgeRoster(event.ID)),
		ResultsPublished: published,
	}
	s.renderPage(w, r, "gallery", data)
}

func (s *Server) webProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.store.SubmissionByID(r.PathValue("id"))
	if err != nil {
		s.webNotFound(w, r, "That project does not exist.")
		return
	}
	event, err := s.store.EventByID(project.EventID)
	if err != nil {
		s.webNotFound(w, r, "That project does not exist.")
		return
	}

	principal, signedIn := auth.PrincipalFromContext(r.Context())
	isJudge := false
	assigned := false
	if signedIn {
		user, err := s.store.UserByID(principal.UserID)
		if err == nil {
			isJudge = user.Role == domain.RoleJudge
			assigned = s.store.IsAssigned(project.EventID, principal.UserID, project.ID)
		}
	}

	trackName := ""
	if track, err := s.store.TrackByID(project.TrackID); err == nil {
		trackName = track.Name
	}
	teamName := ""
	if team, err := s.store.TeamByID(project.TeamID); err == nil {
		teamName = team.Name
	}

	comments := s.store.Comments(project.ID, false)
	type comment struct{ AuthorName, Body string }
	rendered := make([]comment, 0, len(comments))
	for _, item := range comments {
		author := "anonymous"
		if user, err := s.store.UserByID(item.AuthorID); err == nil {
			author = user.DisplayName
		}
		rendered = append(rendered, comment{AuthorName: author, Body: item.Body})
	}

	var rubric domain.Rubric
	active, rubricErr := s.store.ActiveRubric(project.EventID, project.TrackID)
	if rubricErr == nil {
		rubric = active
	}
	reviewCriteria := map[string]int{}
	reviewComment := ""
	reviewSubmitted := false
	if isJudge && assigned {
		if review, err := s.store.ReviewForJudgeProject(principal.UserID, project.ID); err == nil {
			reviewCriteria = review.Criteria
			reviewComment = review.Comment
			reviewSubmitted = review.Submitted
		}
	}

	// Only a judge sees the scoring form, and only for a project assigned to
	// them. The POST handler re-checks the assignment independently, so hiding
	// the form is presentation, not the control.
	canReview := isJudge && assigned && !event.ResultsPublished
	canComment := signedIn && !isJudge

	scale := make([]int, 0, 5)
	for value := rubric.MinScale(); value <= rubric.MaxScale(); value++ {
		scale = append(scale, value)
	}
	if len(scale) == 0 {
		scale = []int{1, 2, 3, 4, 5}
	}

	data := s.pageData(r, project.Title, "event")
	data.Event = event
	data.Page = struct {
		Project         domain.Submission
		EventSlug       string
		TrackName       string
		SubmittedOn     string
		TeamName        string
		ReviewCount     int
		Comments        []comment
		CanComment      bool
		CanReview       bool
		Criteria        []domain.RubricCriterion
		Scale           []int
		Existing        map[string]int
		ReviewComment   string
		ReviewSubmitted bool
	}{
		Project: project, EventSlug: event.Slug, TrackName: trackName, TeamName: teamName,
		SubmittedOn: submittedOn(project),
		ReviewCount: len(s.store.ReviewsForProject(project.ID)),
		Comments:    rendered, CanComment: canComment, CanReview: canReview,
		Criteria: rubric.Criteria, Scale: scale, Existing: reviewCriteria,
		ReviewComment: reviewComment, ReviewSubmitted: reviewSubmitted,
	}
	s.renderPage(w, r, "project", data)
}

func (s *Server) webLogin(w http.ResponseWriter, r *http.Request) {
	data := s.pageData(r, "Sign in", "login")
	data.Page = struct{ Email string }{}
	s.renderPage(w, r, "login", data)
}

func (s *Server) webJudge(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// The console is the judge surface, so it checks the role rather than mere
	// authentication. The JSON route for the same data is gated on
	// ViewAssignments, and these must agree.
	caller, err := s.store.UserByID(principal.UserID)
	if err != nil {
		s.webNotFound(w, r, "No such account.")
		return
	}
	if caller.Role != domain.RoleJudge && !caller.Role.IsStaff() {
		s.webForbidden(w, r)
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		s.webNotFound(w, r, "That event does not exist.")
		return
	}
	summary, err := judging.Normalize(s.store.AllReviews(event.ID), s.eventWeights(event.ID))
	if err != nil {
		// A panel that cannot be normalized is not an error worth a 500 here;
		// the judge still needs their batch.
		s.logger.Warn("judge console could not normalize the panel", "error", err)
		summary = judging.Summary{Projects: []judging.ProjectResult{}, Judges: []judging.JudgeDiagnostic{}}
	}

	assignments := s.store.ListAssignments(event.ID, principal.UserID)
	type assignment struct {
		ProjectID, ProjectTitle, TrackName string
		ReviewSubmitted                    bool
	}
	rows := make([]assignment, 0, len(assignments))
	done := 0
	for _, item := range assignments {
		title := ""
		if project, err := s.store.SubmissionByID(item.ProjectID); err == nil {
			title = project.Title
		}
		track := ""
		if project, err := s.store.SubmissionByID(item.ProjectID); err == nil {
			if record, err := s.store.TrackByID(project.TrackID); err == nil {
				track = record.Name
			}
		}
		submitted := false
		if review, err := s.store.ReviewForJudgeProject(item.JudgeID, item.ProjectID); err == nil {
			submitted = review.Submitted
			if submitted {
				done++
			}
		}
		rows = append(rows, assignment{ProjectID: item.ProjectID, ProjectTitle: title, TrackName: track, ReviewSubmitted: submitted})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ProjectTitle < rows[j].ProjectTitle })

	byProject := make(map[string]int)
	for _, project := range summary.Projects {
		byProject[project.ProjectID] = project.ReviewCount
	}
	type scoreRow struct {
		ProjectID, ProjectTitle, RawScore, NormalizedScore string
		ProjectReviewCount                                 int
	}
	scores := make([]scoreRow, 0, len(summary.Reviews))
	for _, review := range summary.Reviews {
		if review.JudgeID != principal.UserID {
			continue
		}
		title := ""
		if project, err := s.store.SubmissionByID(review.ProjectID); err == nil {
			title = project.Title
		}
		scores = append(scores, scoreRow{
			ProjectID: review.ProjectID, ProjectTitle: title,
			RawScore:           fmt.Sprintf("%.1f", review.RawScore),
			NormalizedScore:    fmt.Sprintf("%.1f", review.NormalizedScore),
			ProjectReviewCount: byProject[review.ProjectID],
		})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].ProjectTitle < scores[j].ProjectTitle })

	low := false
	for _, diagnostic := range summary.Judges {
		if diagnostic.JudgeID == principal.UserID && diagnostic.LowInformation {
			low = true
		}
	}

	data := s.pageData(r, "Judging", "judge")
	data.Event = event
	data.EventSlug = event.Slug
	data.EventName = event.Name
	data.Page = struct {
		Assignments    []assignment
		Reviews        []scoreRow
		Total, Done    int
		Pending        int
		LowInformation bool
	}{
		Assignments: rows, Reviews: scores,
		Total: len(rows), Done: done, Pending: len(rows) - done,
		LowInformation: low,
	}
	s.renderPage(w, r, "judge", data)
}

func (s *Server) webOrganizer(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil || !user.Role.IsStaff() {
		s.webForbidden(w, r)
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		s.webNotFound(w, r, "No event to administer.")
		return
	}
	s.renderOrganizer(w, r, event)
}

func (s *Server) webResults(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil || !user.Role.IsStaff() {
		s.webForbidden(w, r)
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		s.webNotFound(w, r, "No event to administer.")
		return
	}
	summary, err := judging.Normalize(s.store.AllReviews(event.ID), s.eventWeights(event.ID))
	if err != nil {
		data := s.pageData(r, "Results", "organizer")
		data.Notice = "This panel cannot be normalized yet: " + err.Error()
		data.NoticeKind = "bad"
		data.Page = struct{}{}
		s.renderPage(w, r, "results", data)
		return
	}
	separable := 0
	// Titles are resolved here rather than in the template so the standing
	// table reads as projects rather than as database identifiers.
	titles := make(map[string]string, len(summary.Projects))
	tracks := make(map[string]string, len(summary.Projects))
	for _, project := range summary.Projects {
		if project.Separable {
			separable++
		}
		title, track := project.ProjectID, ""
		if submission, err := s.store.SubmissionByID(project.ProjectID); err == nil {
			if submission.Title != "" {
				title = submission.Title
			}
			if record, err := s.store.TrackByID(submission.TrackID); err == nil {
				track = record.Name
			}
		}
		titles[project.ProjectID] = title
		tracks[project.ProjectID] = track
	}
	data := s.pageData(r, "Results", "organizer")
	data.Event = event
	data.Page = struct {
		Event          domain.Event
		Summary        judging.Summary
		Titles         map[string]string
		Tracks         map[string]string
		SeparableCount int
	}{Event: event, Summary: summary, Titles: titles, Tracks: tracks, SeparableCount: separable}
	s.renderPage(w, r, "results", data)
}

func (s *Server) webAccount(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil {
		s.webNotFound(w, r, "No such account.")
		return
	}
	data := s.pageData(r, "Account", "")
	data.Page = struct {
		User         domain.User
		SessionCount int
	}{
		User:         user,
		SessionCount: len(s.store.ListSessions(user.ID)),
	}
	s.renderPage(w, r, "account", data)
}

// ---- form posts ------------------------------------------------------------

func (s *Server) webLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.webFormError(w, r, "login", "", "That form could not be read.")
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	user, err := s.store.UserByEmail(email)
	if err != nil || !auth.VerifyPassword(user.PasswordHash, password) {
		s.webFormError(w, r, "login", email, "That email and password do not match an account.")
		return
	}
	if user.State != "" && user.State != domain.AccountActive && user.State != domain.AccountDeletionPending {
		s.webFormError(w, r, "login", email, "That account is not active.")
		return
	}
	token, err := s.tokens.Issue(user)
	if err != nil {
		s.webFormError(w, r, "login", email, "A session could not be created.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.SessionCookieName, Value: token, Path: "/",
		HttpOnly: true, Secure: s.cfg.IsProduction(), SameSite: http.SameSiteLaxMode,
		MaxAge: s.cfg.SessionTTLHours * 3600,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) webLogoutPost(w http.ResponseWriter, r *http.Request) {
	s.logout(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// submittedOn renders a submission date, tolerating a draft that was never
// submitted.
func submittedOn(project domain.Submission) string {
	if project.SubmittedAt == nil {
		return "not submitted"
	}
	return project.SubmittedAt.UTC().Format("2 Jan 2006")
}

// eventSlugFor resolves an event id to its public slug, for building a link
// back into a console that is addressed by slug.
func eventSlugFor(portal *store.Store, eventID string) string {
	if event, err := portal.EventByID(eventID); err == nil {
		return event.Slug
	}
	return ""
}

func (s *Server) webSaveReview(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.webNotFound(w, r, "That form could not be read.")
		return
	}
	projectID := r.PathValue("id")
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		s.webNotFound(w, r, "That project does not exist.")
		return
	}
	// The same three checks the API applies, re-derived here rather than
	// delegated, so the form path cannot drift from the JSON path.
	if !s.store.IsAssigned(project.EventID, principal.UserID, projectID) {
		s.webForbidden(w, r)
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil || user.Role != domain.RoleJudge {
		s.webForbidden(w, r)
		return
	}
	rubric, err := s.store.ActiveRubric(project.EventID, project.TrackID)
	if err != nil {
		s.webFormError(w, r, "project", projectID, "There is no published rubric for this project yet.")
		return
	}

	criteria := make(map[string]int, len(rubric.Criteria))
	for _, criterion := range rubric.Criteria {
		raw := strings.TrimSpace(r.FormValue("criterion_" + criterion.Key))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			s.webFormError(w, r, "project", projectID, "A score was not a number.")
			return
		}
		criteria[criterion.Key] = value
	}
	if err := rubric.ValidateScores(criteria); err != nil {
		s.webFormError(w, r, "project", projectID, cleanValidation(err.Error()))
		return
	}

	submitted := r.FormValue("submitted") == "true"
	now := s.now().UTC()
	review := domain.Review{
		ID: domain.NewID("rev"), EventID: project.EventID, JudgeID: principal.UserID,
		ProjectID: projectID, Criteria: criteria, RubricID: rubric.ID,
		RubricVersion: rubric.Version, Comment: strings.TrimSpace(r.FormValue("comment")),
		Submitted: submitted, UpdatedAt: now,
	}
	if submitted {
		review.SubmittedAt = &now
	}
	if _, err := s.store.SaveReview(review); err != nil {
		// A submitted review is immutable. That is the control that stops a
		// judge moving a project after seeing the standings, so it gets its own
		// status rather than being flattened into a generic 422.
		if errors.Is(err, domain.ErrConflict) {
			s.errorPage(w, r, http.StatusConflict, "This review is locked",
				"A submitted review cannot be changed. That is deliberate: it is what stops a judge from moving a project after seeing where it stands.")
			return
		}
		s.webFormError(w, r, "project", projectID, cleanValidation(err.Error()))
		return
	}
	http.Redirect(w, r, "/judge/"+eventSlugFor(s.store, project.EventID), http.StatusSeeOther)
}

func (s *Server) webPostComment(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	projectID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		s.webNotFound(w, r, "That form could not be read.")
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		s.webFormError(w, r, "project", projectID, "A comment cannot be empty.")
		return
	}
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		s.webNotFound(w, r, "That project does not exist.")
		return
	}
	_, _ = s.store.CreateComment(domain.Comment{
		ID: domain.NewID("cmt"), EventID: project.EventID, ProjectID: projectID,
		AuthorID: principal.UserID, Body: body, Status: domain.CommentVisible,
		CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC(),
	})
	http.Redirect(w, r, "/projects/"+projectID, http.StatusSeeOther)
}

func (s *Server) webPublish(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil || !user.Role.IsStaff() {
		s.webForbidden(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.webNotFound(w, r, "That form could not be read.")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		s.webNotFound(w, r, "No event to administer.")
		return
	}
	publish := r.FormValue("action") != "unpublish"
	_, err = s.store.SetResultsPublished(event.ID, publish, publish, s.now())
	if err != nil {
		s.renderOrganizer(w, r, event)
		return
	}
	http.Redirect(w, r, "/organizer/"+event.Slug, http.StatusSeeOther)
}

// ---- shared helpers --------------------------------------------------------

func (s *Server) renderOrganizer(w http.ResponseWriter, r *http.Request, event domain.Event) {
	summary, err := judging.Normalize(s.store.AllReviews(event.ID), s.eventWeights(event.ID))
	if err != nil {
		summary = judging.Summary{Projects: []judging.ProjectResult{}, Judges: []judging.JudgeDiagnostic{}}
	}
	low := make([]judging.JudgeDiagnostic, 0, len(summary.Judges))
	for _, diagnostic := range summary.Judges {
		if diagnostic.LowInformation {
			low = append(low, diagnostic)
		}
	}
	type panelRow struct {
		DisplayName, Tracks string
		Reviews             int
		LowInformation      bool
	}
	panel := make([]panelRow, 0)
	for _, entry := range s.store.JudgeRoster(event.ID) {
		row := panelRow{}
		if user, err := s.store.UserByID(entry.JudgeID); err == nil {
			row.DisplayName = user.DisplayName
		} else {
			row.DisplayName = entry.JudgeID
		}
		if profile, err := s.store.JudgeProfile(entry.JudgeID); err == nil {
			row.Tracks = strings.Join(profile.Tracks, ", ")
		}
		for _, diagnostic := range summary.Judges {
			if diagnostic.JudgeID == entry.JudgeID {
				row.Reviews = diagnostic.Reviews
				row.LowInformation = diagnostic.LowInformation
			}
		}
		panel = append(panel, row)
	}
	sort.Slice(panel, func(i, j int) bool { return panel[i].DisplayName < panel[j].DisplayName })

	data := s.pageData(r, "Organizer", "organizer")
	data.Event = event
	data.Page = struct {
		Event            domain.Event
		Progress         domain.Progress
		Duplicates       []domain.DuplicateFlag
		LowInformation   []judging.JudgeDiagnostic
		Panel            []panelRow
		ProjectCount     int
		ResultsPublished bool
	}{
		Event:            event,
		Progress:         s.store.Progress(event.ID),
		Duplicates:       s.store.ListDuplicates(event.ID, domain.DuplicateOpen),
		LowInformation:   low,
		Panel:            panel,
		ProjectCount:     len(s.store.ListSubmissions(event.ID, "", "")),
		ResultsPublished: event.ResultsPublished,
	}
	s.renderPage(w, r, "organizer", data)
}

// errorPage renders a standalone page with a real status code. It never falls
// back to another page's template: sharing one would mean a missing field
// turns a clean 404 into a template execution error and a 500.
func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int, heading, detail string) {
	data := s.pageData(r, heading, "")
	data.Page = struct{ Heading, Detail string }{Heading: heading, Detail: detail}
	tmpl, err := s.pageTemplate("error")
	if err != nil {
		http.Error(w, detail, status)
		return
	}
	var buffer bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buffer, "shell", data); err != nil {
		s.logger.Error("could not render the error page", "error", err)
		http.Error(w, detail, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buffer.Bytes())
}

func (s *Server) webNotFound(w http.ResponseWriter, r *http.Request, message string) {
	s.errorPage(w, r, http.StatusNotFound, "Not found", message)
}

func (s *Server) webForbidden(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusForbidden, "Not permitted",
		"That page is not available to your role. The same refusal is enforced on the JSON routes, so it is not a matter of which link you use.")
}

// webFormError re-renders the form with an explanation rather than losing the
// visitor's place. The form pages are static templates, so a rejected form
// falls back to the error page with a 422 rather than re-exposing a POST target.
func (s *Server) webFormError(w http.ResponseWriter, r *http.Request, page, context, message string) {
	s.errorPage(w, r, http.StatusUnprocessableEntity, "Check the form", message)
	_ = page
	_ = context
}

func cleanValidation(message string) string {
	message = strings.TrimPrefix(message, "validation: ")
	return strings.ToUpper(message[:1]) + message[1:]
}

// staticHandler serves the embedded stylesheet.
// webStatic serves one embedded stylesheet.
//
// The token layer has to load before the layers that consume it, so the shell
// links five files rather than concatenating them. Each is a separate route so
// the browser can cache them independently and revalidate one without the rest.
func (s *Server) webStatic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !embeddedStylesheets[name] {
		http.NotFound(w, r)
		return
	}
	s.serveEmbedded(w, r, "webassets/static/"+name, "text/css; charset=utf-8")
}

// webBrand serves the approved brand assets from section 10.2.
//
// The allowlist is deliberate: the embed contains a fixture with participant
// data, and a path-traversal guess must not be able to read it. Only the two
// approved files are reachable, and neither is user-supplied.
func (s *Server) webBrand(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name != "logo.svg" && name != "icon.svg" {
		http.NotFound(w, r)
		return
	}
	s.serveEmbedded(w, r, "webassets/brand/"+name, "image/svg+xml")
}

// embeddedStylesheets is the set of stylesheets the shell may request.
var embeddedStylesheets = map[string]bool{
	"tokens.css":     true,
	"base.css":       true,
	"components.css": true,
	"shell.css":      true,
	"apps.css":       true,
	"app.css":        true, // superseded by the layers above, still routed
}

func (s *Server) serveEmbedded(w http.ResponseWriter, r *http.Request, path, contentType string) {
	file, err := webAssets.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

// registerWeb mounts the HTML frontend. It returns the handler so the caller
// can decide how to compose it with the API mux.
func (s *Server) registerWeb(mux *routeMux) {
	// The layout needs the caller's identity to render the right navigation,
	// so pages go through optionalAuth and then re-check permissions per page.
	page := func(handler http.HandlerFunc) http.Handler { return s.optionalAuth(handler) }

	mux.Handle("GET /static/{name}", http.HandlerFunc(s.webStatic))
	mux.Handle("GET /brand/{name}", http.HandlerFunc(s.webBrand))
	mux.Handle("GET /{$}", page(s.webHome))
	mux.Handle("GET /login", page(s.webLogin))
	mux.Handle("POST /login", s.guard(ratelimitLogin, http.HandlerFunc(s.webLoginPost)))
	mux.Handle("POST /logout", s.guard(ratelimitAccount, http.HandlerFunc(s.webLogoutPost)))
	mux.Handle("GET /judge/{slug}", page(s.webJudge))
	mux.Handle("POST /judge/{slug}/projects/{id}", s.guard(ratelimitWrite, s.optionalAuth(s.webSaveReview)))
	mux.Handle("GET /organizer/{slug}", page(s.webOrganizer))
	mux.Handle("GET /organizer/{slug}/results", page(s.webResults))
	mux.Handle("POST /organizer/{slug}/publish", s.guard(ratelimitWrite, s.optionalAuth(s.webPublish)))
	mux.Handle("GET /events/{slug}", page(s.webGallery))
	mux.Handle("GET /projects/{id}", page(s.webProject))
	mux.Handle("POST /projects/{id}/comment", s.guard(ratelimitComment, s.optionalAuth(s.webPostComment)))
	mux.Handle("GET /account", page(s.webAccount))
}

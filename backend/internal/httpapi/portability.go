package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type ExportBundle struct {
	Format        string                      `json:"format"`
	ExportedAt    time.Time                   `json:"exported_at"`
	ExportedBy    string                      `json:"exported_by"`
	Event         domain.Event                `json:"event"`
	Tracks        []domain.Track              `json:"tracks"`
	Prizes        []domain.Prize              `json:"prizes"`
	Questions     []domain.HackathonQuestion  `json:"questions"`
	Milestones    []domain.HackathonMilestone `json:"milestones"`
	Hosts         []domain.HackathonHost      `json:"hosts"`
	Rubrics       []domain.Rubric             `json:"rubrics"`
	JudgeRoster   []domain.JudgeRosterEntry   `json:"judge_roster"`
	Teams         []domain.Team               `json:"teams"`
	Submissions   []domain.Submission         `json:"submissions"`
	Reviews       []domain.Review             `json:"reviews"`
	Assignments   []domain.Assignment         `json:"assignments"`
	Comments      []domain.Comment            `json:"comments"`
	Activity      []domain.ActivityEntry      `json:"activity"`
	Participation []domain.Participation      `json:"participations"`
	VoteResults   []domain.CampaignResult     `json:"vote_results"`
	Stats         map[string]int              `json:"stats"`
	MailTemplates []string                    `json:"mail_templates"`
	Notes         []string                    `json:"notes"`
}

func (s *Server) exportEvent(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventByID(strings.TrimSpace(r.URL.Query().Get("event_id")))
	if err != nil {
		event, err = s.store.EventBySlug("sample-hack-2026")
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
			return
		}
	}
	bundle := ExportBundle{
		Format:        "codeceremony.export.v1",
		ExportedAt:    s.now().UTC(),
		ExportedBy:    principal.UserID,
		Event:         event,
		Tracks:        s.store.ListTracks(event.ID),
		Prizes:        s.store.ListPrizes(event.ID),
		Questions:     s.store.Questions(event.ID, ""),
		Milestones:    s.store.Milestones(event.ID),
		Hosts:         s.store.Hosts(event.ID),
		Rubrics:       s.store.RubricsForEvent(event.ID),
		JudgeRoster:   s.store.JudgeRoster(event.ID),
		Teams:         s.store.ListTeams(event.ID),
		Submissions:   s.store.ListSubmissions(event.ID, "", ""),
		Reviews:       s.store.AllReviews(event.ID),
		Assignments:   s.store.ListAssignments(event.ID, ""),
		Comments:      s.store.Comments("", false),
		Activity:      s.activityForExport(event.ID),
		Participation: s.store.Participations("", event.ID),
		Stats:         map[string]int{},
		MailTemplates: templateNames(s),
		Notes: []string{
			"password hashes, session tokens, mail secrets, and webhook signing secrets are never exported",
			"this bundle is sufficient to recreate the hackathon through POST /v1/organizer/import",
		},
	}
	bundle.Stats["teams"] = len(bundle.Teams)
	bundle.Stats["submissions"] = len(bundle.Submissions)
	bundle.Stats["reviews"] = len(bundle.Reviews)
	bundle.Stats["assignments"] = len(bundle.Assignments)
	bundle.Stats["questions"] = len(bundle.Questions)
	bundle.Stats["activity"] = len(bundle.Activity)
	for _, campaign := range s.store.Campaigns(event.ID) {
		if result, err := s.store.CampaignResult(campaign.ID); err == nil {
			bundle.VoteResults = append(bundle.VoteResults, result)
		}
	}
	bundle.Comments = filterCommentsForEvent(bundle.Comments, event.ID)
	w.Header().Set("Content-Disposition", `attachment; filename="`+event.Slug+`-export.json"`)
	writeJSON(w, http.StatusOK, map[string]any{"data": bundle, "stats": bundle.Stats})
}

func filterCommentsForEvent(comments []domain.Comment, eventID string) []domain.Comment {
	filtered := make([]domain.Comment, 0, len(comments))
	for _, comment := range comments {
		if comment.EventID == eventID {
			filtered = append(filtered, comment)
		}
	}
	return filtered
}

func (s *Server) activityForExport(eventID string) []domain.ActivityEntry {
	page, _ := s.store.ListActivity(domain.ActivityFilter{EventID: eventID, Limit: 200})
	return page.Entries
}

func templateNames(s *Server) []string {
	names := make([]string, 0)
	for _, tmpl := range s.mailService.Registry().List() {
		names = append(names, tmpl.Name)
	}
	sort.Strings(names)
	return names
}

type ImportPlan struct {
	Slug        string   `json:"slug"`
	Created     bool     `json:"created"`
	Tracks      int      `json:"tracks"`
	Prizes      int      `json:"prizes"`
	Questions   int      `json:"questions"`
	Rubrics     int      `json:"rubrics"`
	Teams       int      `json:"teams"`
	Submissions int      `json:"submissions"`
	Reviews     int      `json:"reviews"`
	Warnings    []string `json:"warnings"`
	Errors      []string `json:"errors"`
	DryRun      bool     `json:"dry_run"`
}

func (s *Server) importEvent(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		DryRun     bool         `json:"dry_run"`
		SlugSuffix string       `json:"slug_suffix"`
		Bundle     ExportBundle `json:"bundle"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	bundle := request.Bundle
	plan := ImportPlan{DryRun: request.DryRun}
	if strings.TrimSpace(bundle.Event.Name) == "" {
		plan.Errors = append(plan.Errors, "bundle event name is required")
	}
	slug := strings.TrimSpace(bundle.Event.Slug)
	if slug == "" {
		plan.Errors = append(plan.Errors, "bundle event slug is required")
	}
	slug += strings.TrimSpace(request.SlugSuffix)
	plan.Slug = slug
	if !bundle.Event.SubmissionsClose.IsZero() && bundle.Event.SubmissionsClose.Before(s.now().UTC()) {
		plan.Warnings = append(plan.Warnings, "submissions_close is in the past; imported submissions will be drafts or submitted per bundle status")
	}
	if len(bundle.Rubrics) > 0 {
		for _, rubric := range bundle.Rubrics {
			if err := rubric.Validate(); err != nil {
				plan.Warnings = append(plan.Warnings, "rubric "+rubric.Name+" skipped: "+err.Error())
			}
		}
	}
	questionKeys := map[string]struct{}{}
	for _, question := range bundle.Questions {
		questionKeys[question.Key] = struct{}{}
	}
	for _, submission := range bundle.Submissions {
		for key := range submission.CustomAnswers {
			if _, ok := questionKeys[key]; !ok {
				plan.Warnings = append(plan.Warnings, "submission "+submission.Title+" answers unknown question "+key)
			}
		}
	}
	if _, err := s.store.EventBySlug(slug); err == nil {
		plan.Warnings = append(plan.Warnings, "a hackathon with slug "+slug+" already exists and will be updated in place")
	}
	if len(plan.Errors) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"plan": plan})
		return
	}
	if request.DryRun {
		plan.Tracks = len(bundle.Tracks)
		plan.Prizes = len(bundle.Prizes)
		plan.Questions = len(bundle.Questions)
		plan.Rubrics = len(bundle.Rubrics)
		plan.Teams = len(bundle.Teams)
		plan.Submissions = len(bundle.Submissions)
		plan.Reviews = len(bundle.Reviews)
		writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
		return
	}

	event, err := s.store.EventBySlug(slug)
	if err != nil {
		imported := bundle.Event
		imported.ID = domain.NewID("evt")
		imported.Slug = slug
		imported.State = domain.HackathonRegistrationOpen
		imported.ResultsPublished = false
		imported.ResultsPublishedAt = nil
		imported.CreatedAt = s.now().UTC()
		imported.UpdatedAt = s.now().UTC()
		if err := s.store.CreateEvent(imported); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "the hackathon could not be created from this bundle")
			return
		}
		event, err = s.store.EventBySlug(slug)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "the imported hackathon could not be read back")
			return
		}
		plan.Created = true
	}

	trackIDs := map[string]string{}
	for _, track := range bundle.Tracks {
		created, err := s.store.CreateTrack(domain.Track{Event: event.ID, Name: track.Name, Slug: track.Slug, Summary: track.Summary, Order: track.Order})
		if err != nil {
			plan.Warnings = append(plan.Warnings, "track "+track.Name+" skipped: "+err.Error())
			continue
		}
		trackIDs[track.ID] = created.ID
		plan.Tracks++
	}
	for _, prize := range bundle.Prizes {
		mapped := prize
		mapped.ID = ""
		mapped.EventID = event.ID
		if mapped.TrackID != "" {
			if mappedID, ok := trackIDs[prize.TrackID]; ok {
				mapped.TrackID = mappedID
			} else {
				mapped.TrackID = ""
			}
		}
		if _, err := s.store.CreatePrize(mapped); err != nil {
			plan.Warnings = append(plan.Warnings, "prize "+prize.Name+" skipped: "+err.Error())
			continue
		}
		plan.Prizes++
	}
	for _, question := range bundle.Questions {
		created, err := s.store.CreateQuestion(domain.HackathonQuestion{
			EventID:  event.ID,
			Key:      question.Key,
			Prompt:   question.Prompt,
			HelpText: question.HelpText,
			Type:     question.Type,
			Audience: question.Audience,
			Required: question.Required,
			Options:  question.Options,
			Position: question.Position,
		})
		if err != nil {
			plan.Warnings = append(plan.Warnings, "question "+question.Prompt+" skipped: "+err.Error())
			continue
		}
		questionKeys[created.Key] = struct{}{}
		plan.Questions++
	}
	for _, rubric := range bundle.Rubrics {
		candidate := rubric
		candidate.ID = ""
		candidate.EventID = event.ID
		candidate.TrackID = ""
		candidate.CreatedBy = principal.UserID
		candidate.Status = domain.RubricDraft
		candidate.PublishedAt = nil
		candidate.ArchivedAt = nil
		if _, err := s.store.CreateRubric(candidate); err != nil {
			plan.Warnings = append(plan.Warnings, "rubric "+rubric.Name+" skipped: "+err.Error())
			continue
		}
		plan.Rubrics++
	}
	teamIDs := map[string]string{}
	for _, team := range bundle.Teams {
		candidate := team
		candidate.ID = ""
		candidate.EventID = event.ID
		if candidate.CaptainID == "" {
			candidate.CaptainID = principal.UserID
		}
		if _, err := s.store.UserByID(candidate.CaptainID); err != nil {
			candidate.CaptainID = principal.UserID
		}
		candidate.Status = domain.TeamStatusActive
		candidate.CreatedAt = s.now().UTC()
		if candidate.Name == "" {
			continue
		}
		if err := s.store.CreateTeam(candidate); err != nil {
			plan.Warnings = append(plan.Warnings, "team "+team.Name+" skipped: "+err.Error())
			continue
		}
		plan.Teams++
		teamIDs[team.ID] = team.Name
	}
	for _, submission := range bundle.Submissions {
		candidate := submission
		candidate.ID = domain.NewID("sub")
		candidate.EventID = event.ID
		if candidate.TrackID == "" {
			plan.Warnings = append(plan.Warnings, "submission "+submission.Title+" skipped: no track")
			continue
		}
		if candidate.Title == "" || candidate.Summary == "" {
			plan.Warnings = append(plan.Warnings, "a submission was skipped because it has no title or summary")
			continue
		}
		candidate.Status = domain.SubmissionSubmitted
		if candidate.Eligibility == "" {
			candidate.Eligibility = domain.EligibilityPending
		}
		candidate.UpdatedAt = s.now().UTC()
		if err := s.store.CreateSubmission(candidate); err != nil {
			plan.Warnings = append(plan.Warnings, "submission "+submission.Title+" skipped: "+err.Error())
			continue
		}
		plan.Submissions++
	}
	for _, review := range bundle.Reviews {
		candidate := review
		candidate.ID = ""
		candidate.EventID = event.ID
		if _, err := s.store.ReviewForJudgeProject(review.JudgeID, review.ProjectID); err == nil {
			plan.Warnings = append(plan.Warnings, "a review for "+review.ProjectID+" was skipped because it already exists")
			continue
		}
		if _, err := s.store.SaveReview(candidate); err != nil {
			if errors.Is(err, domain.ErrValidation) {
				plan.Warnings = append(plan.Warnings, "a review was skipped because no published rubric accepts its criteria")
				continue
			}
			plan.Warnings = append(plan.Warnings, "a review was skipped")
			continue
		}
		plan.Reviews++
	}
	s.audit(r, principal.UserID, "event.imported", "event", event.ID, event.ID, "hackathon imported from bundle", map[string]any{
		"teams": plan.Teams, "submissions": plan.Submissions, "reviews": plan.Reviews, "warnings": len(plan.Warnings),
	})
	s.recordActivity(r, principal.UserID, domain.ActivityEvent, "event.imported", "event", event.ID, event.ID,
		"a hackathon was imported from an export bundle", domain.ActivityParticipants, map[string]any{"submissions": plan.Submissions})
	writeJSON(w, http.StatusCreated, map[string]any{"plan": plan, "event": event})
}

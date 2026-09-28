package store

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

type Store struct {
	mu          sync.RWMutex
	users       map[string]domain.User
	usersByMail map[string]string
	events      map[string]domain.Event
	tracks      map[string]domain.Track
	teams       map[string]domain.Team
	submissions map[string]domain.Submission
	assignments map[string]domain.Assignment
	reviews     map[string]domain.Review
}

func New(data seed.Data) *Store {
	store := &Store{
		users:       make(map[string]domain.User, len(data.Users)),
		usersByMail: make(map[string]string, len(data.Users)),
		events:      make(map[string]domain.Event, len(data.Events)),
		tracks:      make(map[string]domain.Track, len(data.Tracks)),
		teams:       make(map[string]domain.Team, len(data.Teams)),
		submissions: make(map[string]domain.Submission, len(data.Submissions)),
		assignments: make(map[string]domain.Assignment, len(data.Assignments)),
		reviews:     make(map[string]domain.Review, len(data.Reviews)),
	}
	for _, user := range data.Users {
		store.users[user.ID] = user
		store.usersByMail[strings.ToLower(user.Email)] = user.ID
	}
	for _, event := range data.Events {
		store.events[event.ID] = event
	}
	for _, track := range data.Tracks {
		store.tracks[track.ID] = track
	}
	for _, team := range data.Teams {
		store.teams[team.ID] = team
	}
	for _, submission := range data.Submissions {
		store.submissions[submission.ID] = submission
	}
	for _, assignment := range data.Assignments {
		store.assignments[assignment.ID] = assignment
	}
	for _, review := range data.Reviews {
		store.reviews[reviewKey(review.JudgeID, review.ProjectID)] = review
	}
	return store
}

func (s *Store) UserByEmail(email string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usersByMail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return s.users[id], nil
}

func (s *Store) UserByID(id string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (s *Store) EventBySlug(slug string) (domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, event := range s.events {
		if event.Slug == slug {
			return event, nil
		}
	}
	return domain.Event{}, domain.ErrNotFound
}

func (s *Store) EventByID(id string) (domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.events[id]
	if !ok {
		return domain.Event{}, domain.ErrNotFound
	}
	return event, nil
}

func (s *Store) ListEvents() []domain.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Event, 0, len(s.events))
	for _, event := range s.events {
		result = append(result, event)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (s *Store) TrackByID(id string) (domain.Track, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	track, ok := s.tracks[id]
	if !ok {
		return domain.Track{}, domain.ErrNotFound
	}
	return track, nil
}

func (s *Store) TeamByID(id string) (domain.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[id]
	if !ok {
		return domain.Team{}, domain.ErrNotFound
	}
	return team, nil
}

func (s *Store) SubmissionByID(id string) (domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.ErrNotFound
	}
	return cloneSubmission(submission), nil
}

func (s *Store) ListSubmissions(eventID, query, trackID string) []domain.Submission {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]domain.Submission, 0)
	for _, submission := range s.submissions {
		if submission.EventID != eventID || !isPublicSubmission(submission.Status) {
			continue
		}
		if trackID != "" && submission.TrackID != trackID {
			continue
		}
		if query != "" && !submissionMatches(submission, query) {
			continue
		}
		result = append(result, cloneSubmission(submission))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Title == result[j].Title {
			return result[i].ID < result[j].ID
		}
		return result[i].Title < result[j].Title
	})
	return result
}

func (s *Store) CreateSubmission(submission domain.Submission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if submission.ID == "" {
		submission.ID = domain.NewID("sub")
	}
	if _, exists := s.submissions[submission.ID]; exists {
		return domain.ErrAlreadyExists
	}
	if _, exists := s.events[submission.EventID]; !exists {
		return domain.ErrNotFound
	}
	if _, exists := s.teams[submission.TeamID]; !exists {
		return domain.ErrNotFound
	}
	if submission.Version == 0 {
		submission.Version = 1
	}
	if submission.Status == "" {
		submission.Status = domain.SubmissionDraft
	}
	if submission.UpdatedAt.IsZero() {
		submission.UpdatedAt = time.Now().UTC()
	}
	s.submissions[submission.ID] = cloneSubmission(submission)
	return nil
}

func (s *Store) UpdateSubmission(id string, update func(domain.Submission) (domain.Submission, error)) (domain.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.ErrNotFound
	}
	updated, err := update(cloneSubmission(submission))
	if err != nil {
		return domain.Submission{}, err
	}
	updated.ID = id
	updated.Version = submission.Version + 1
	updated.UpdatedAt = time.Now().UTC()
	s.submissions[id] = cloneSubmission(updated)
	return cloneSubmission(updated), nil
}

func (s *Store) IsAssigned(eventID, judgeID, projectID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID && assignment.JudgeID == judgeID && assignment.ProjectID == projectID {
			return true
		}
	}
	return false
}

func (s *Store) ReviewsForJudge(eventID, judgeID string) []domain.Review {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Review, 0)
	for _, review := range s.reviews {
		if review.EventID == eventID && review.JudgeID == judgeID {
			result = append(result, cloneReview(review))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProjectID < result[j].ProjectID })
	return result
}

func (s *Store) ReviewForJudgeProject(judgeID, projectID string) (domain.Review, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	review, ok := s.reviews[reviewKey(judgeID, projectID)]
	if !ok {
		return domain.Review{}, domain.ErrNotFound
	}
	return cloneReview(review), nil
}

func (s *Store) SaveReview(review domain.Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review.ID == "" {
		review.ID = domain.NewID("rev")
	}
	if _, exists := s.events[review.EventID]; !exists {
		return domain.ErrNotFound
	}
	if _, exists := s.submissions[review.ProjectID]; !exists {
		return domain.ErrNotFound
	}
	review.UpdatedAt = time.Now().UTC()
	s.reviews[reviewKey(review.JudgeID, review.ProjectID)] = cloneReview(review)
	return nil
}

func (s *Store) Progress(eventID string) domain.Progress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	progress := domain.Progress{EventID: eventID}
	for _, submission := range s.submissions {
		if submission.EventID != eventID || !isPublicSubmission(submission.Status) {
			continue
		}
		progress.Submissions++
		progress.EligibleSubmissions++
	}
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID {
			progress.Assignments++
		}
	}
	for _, review := range s.reviews {
		if review.EventID != eventID {
			continue
		}
		if review.Submitted {
			progress.ReviewsCompleted++
		} else {
			progress.ReviewsStarted++
		}
	}
	progress.ReviewsPending = progress.Assignments - progress.ReviewsCompleted
	if progress.ReviewsPending < 0 {
		progress.ReviewsPending = 0
	}
	return progress
}

func (s *Store) AllReviews(eventID string) []domain.Review {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Review, 0)
	for _, review := range s.reviews {
		if review.EventID == eventID {
			result = append(result, cloneReview(review))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectID == result[j].ProjectID {
			return result[i].JudgeID < result[j].JudgeID
		}
		return result[i].ProjectID < result[j].ProjectID
	})
	return result
}

func isPublicSubmission(status domain.SubmissionStatus) bool {
	return status == domain.SubmissionSubmitted || status == domain.SubmissionLocked
}

func submissionMatches(submission domain.Submission, query string) bool {
	values := []string{submission.Title, submission.Summary, strings.Join(submission.Tags, " ")}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func reviewKey(judgeID, projectID string) string {
	return judgeID + ":" + projectID
}

func cloneSubmission(submission domain.Submission) domain.Submission {
	copySubmission := submission
	copySubmission.Tags = append([]string(nil), submission.Tags...)
	if submission.CustomAnswers != nil {
		copySubmission.CustomAnswers = make(map[string]string, len(submission.CustomAnswers))
		for key, value := range submission.CustomAnswers {
			copySubmission.CustomAnswers[key] = value
		}
	}
	return copySubmission
}

func cloneReview(review domain.Review) domain.Review {
	copyReview := review
	copyReview.Criteria = make(map[string]int, len(review.Criteria))
	for key, value := range review.Criteria {
		copyReview.Criteria[key] = value
	}
	return copyReview
}

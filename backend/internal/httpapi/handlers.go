package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	eventID, ok := s.queryEvent(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="codeceremony-export.csv"`)
	writer := csv.NewWriter(w)
	defer writer.Flush()
	if err := writer.Write([]string{"event_id", "project_id", "project_title", "judge_id", "functionality", "quality", "innovation", "comment"}); err != nil {
		return
	}
	for _, review := range s.store.AllReviews(eventID) {
		project, err := s.store.SubmissionByID(review.ProjectID)
		if err != nil {
			continue
		}
		record := []string{
			review.EventID,
			review.ProjectID,
			project.Title,
			review.JudgeID,
			strconv.Itoa(review.Criteria["functionality"]),
			strconv.Itoa(review.Criteria["quality"]),
			strconv.Itoa(review.Criteria["innovation"]),
			review.Comment,
		}
		if err := writer.Write(record); err != nil {
			return
		}
	}
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

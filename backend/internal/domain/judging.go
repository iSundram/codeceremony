package domain

import "time"

type JudgeProfile struct {
	UserID   string   `json:"user_id"`
	Bio      string   `json:"bio"`
	Tracks   []string `json:"tracks"`
	Capacity int      `json:"capacity"`
	Active   bool     `json:"active"`
}

type ConflictDeclaration struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	JudgeID   string    `json:"judge_id"`
	ProjectID string    `json:"project_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type AssignmentStrategy string

const (
	AssignmentManual   AssignmentStrategy = "manual"
	AssignmentBalanced AssignmentStrategy = "balanced"
	AssignmentBatch    AssignmentStrategy = "batch"
)

package domain

import "time"

type Track struct {
	ID      string `json:"id"`
	Event   string `json:"event_id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Summary string `json:"summary,omitempty"`
	Order   int    `json:"order"`
}

type Prize struct {
	ID          string    `json:"id"`
	EventID     string    `json:"event_id"`
	TrackID     string    `json:"track_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Rank        int       `json:"rank"`
	CreatedAt   time.Time `json:"created_at"`
}

// ScopeOrDefault reports the team scope an event uses when none is configured.
func (e Event) ScopeOrDefault() TeamScope {
	if e.AllowGlobalTeams {
		return TeamScopeGlobal
	}
	return TeamScopeHackathon
}

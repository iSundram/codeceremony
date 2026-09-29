package httpapi

import (
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"sort"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// ballotOptions is the list of projects a voter may choose, in the order that
// voter should see them.
//
// # Why the order is randomized
//
// A fixed order is a quiet channel. If the ballot always lists the strongest
// projects first, then a voter who simply takes the top of the list is applying
// a ranking the organizer never asked for, and any campaign being gamed for
// favour can be gamed by asking its audience to tick a visible pattern. It
// also makes collusion easier to coordinate: "vote for everything in the top
// five" is only workable if the top five is the same for everybody.
//
// # Why it is deterministic
//
// The order is derived from the campaign and the voter, not from a random
// source per request. Two properties follow, and both matter:
//
//   - Refreshing the page does not reshuffle the list under the voter's cursor.
//   - A voter cannot re-roll until they get an order they like, which would
//     reintroduce exactly the bias the shuffle removes.
//
// Different voters see different orders, which is the whole point; the same
// voter always sees the same one.
func (s *Server) ballotOptions(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	campaign, err := s.openCampaignFor(r, event)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no open vote campaign for this event")
		return
	}

	// Eligible projects are the ballot. A campaign that only accepts eligible
	// submissions must not show the ineligible ones at all, or the voter can
	// pick something that is then silently discarded.
	projects := s.store.ListSubmissions(event.ID, "", "")
	options := make([]ballotOption, 0, len(projects))
	for _, project := range projects {
		if campaign.RequireEligible && project.Eligibility == domain.EligibilityIneligible {
			continue
		}
		option := ballotOption{ProjectID: project.ID, Title: project.Title, Summary: project.Summary}
		if track, err := s.store.TrackByID(project.TrackID); err == nil {
			option.TrackName = track.Name
		}
		if team, err := s.store.TeamByID(project.TeamID); err == nil {
			option.TeamName = team.Name
		}
		options = append(options, option)
	}

	shuffleForVoter(options, campaign.ID, principal.UserID)
	chosen := make(map[string]bool)
	for _, ballot := range s.store.BallotsForUser(campaign.ID, principal.UserID) {
		chosen[ballot.ProjectID] = true
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": options,
		"meta": map[string]any{
			"campaign_id":          campaign.ID,
			"name":                 campaign.Name,
			"max_choices_per_user": campaign.MaxChoicesPerUser,
			"remaining_choices":    remainingChoices(campaign, chosen),
			"eligible_projects":    len(options),
			"order":                "randomized per voter, stable across refreshes",
			"already_chosen":       chosenList(chosen),
		},
	})
}

type ballotOption struct {
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	TrackName string `json:"track_name,omitempty"`
	TeamName  string `json:"team_name,omitempty"`
}

func remainingChoices(campaign domain.VoteCampaign, chosen map[string]bool) int {
	remaining := campaign.MaxChoicesPerUser - len(chosen)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func chosenList(chosen map[string]bool) []string {
	list := make([]string, 0, len(chosen))
	for projectID, isChosen := range chosen {
		if isChosen {
			list = append(list, projectID)
		}
	}
	sort.Strings(list)
	return list
}

// openCampaignFor resolves which campaign a ballot request refers to. An
// explicit campaign_id wins; otherwise the single open campaign is used.
func (s *Server) openCampaignFor(r *http.Request, event domain.Event) (domain.VoteCampaign, error) {
	if requested := strings.TrimSpace(r.URL.Query().Get("campaign_id")); requested != "" {
		for _, campaign := range s.store.Campaigns(event.ID) {
			if campaign.ID == requested {
				return campaign, nil
			}
		}
		return domain.VoteCampaign{}, domain.ErrNotFound
	}
	for _, campaign := range s.store.Campaigns(event.ID) {
		if open, _ := campaign.VotingOpen(s.now().UTC()); open {
			return campaign, nil
		}
	}
	return domain.VoteCampaign{}, domain.ErrNotFound
}

// shuffleForVoter orders options deterministically for one voter.
//
// The permutation comes from sorting on a keyed digest of each project id. A
// digest rather than a seeded PRNG because a PRNG draws depend on the order it
// is fed, so adding or removing a project would reshuffle everything; sorting
// on per-item keys means adding a project leaves every other voter's order
// alone.
func shuffleForVoter(options []ballotOption, campaignID, voterID string) {
	salt := campaignID + "\x00" + voterID
	sort.SliceStable(options, func(i, j int) bool {
		return voterKey(salt, options[i].ProjectID) < voterKey(salt, options[j].ProjectID)
	})
}

func voterKey(salt, projectID string) uint64 {
	sum := sha256.Sum256([]byte(salt + "\x00" + projectID))
	return binary.BigEndian.Uint64(sum[:8])
}

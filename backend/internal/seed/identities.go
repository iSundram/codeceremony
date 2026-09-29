package seed

import (
	"sort"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Identity is a seeded account paired with a stable, documented session token.
//
// The DOGFOOD acceptance checker never logs in. Logging in is the one thing no
// two stacks do alike, so a portal hands over a working auth header and the
// checker attaches it verbatim. That only works if the token is the same on
// every boot, which is what these are: fixed values written into .dogfood.toml
// at the repository root.
//
// These tokens grant privileged access, so they are only ever minted when
// seeding is enabled. Configuration.Load refuses to enable seeding in
// production, and IssueSeededIdentities refuses to run without an explicit
// opt-in flag.
type Identity struct {
	// Label is the key used in the .dogfood.toml [auth] table.
	Label string
	// Candidates are the account ids this label can authenticate as, in
	// preference order. A portal seeded from the shared fixtures resolves to
	// the fixture panel; a portal seeded from the built-in demo data resolves
	// to its own. Pinning a single id would leave one of those two
	// configurations unable to start.
	Candidates []string
	// Token is the fixed session token. It is a credential for a demo portal
	// seeded with invented data, not a secret.
	Token string
	// Role documents the intended role so the printed banner and the
	// documentation cannot drift from reality.
	Role domain.Role
	// Email is filled in from whichever account the label resolved to, so the
	// banner and the login page never quote an address that does not exist.
	Email string
}

// Resolved is an Identity bound to the account it actually authenticates as.
type Resolved struct {
	Identity
	UserID string
}

// SeededIdentities is the fixed credential set for the seeded portal.
//
// Tokens are long enough to be unguessable by construction rather than by
// secrecy, and are namespaced with a `cc_` prefix so they are greppable and
// obviously not production credentials.
//
// The two judge labels prefer a fixture judge and fall back to the built-in
// demo judge. judge_a is jdg_24 (Diego Herrera), the busiest judge in the
// fixture panel at eleven reviews, which makes for a meaningful console. judge_b
// is jdg_07 (Iva Petrova), the judge who gave every project they reviewed the
// same score: that is the fixture's low-information case, and binding the
// peer-isolation identity to it means the calibration warning is visible the
// moment someone opens the console.
func SeededIdentities() []Identity {
	return []Identity{
		{Label: "organizer", Candidates: []string{"organizer"}, Token: "cc_org_7f2a1b9d4e6c8a0f", Role: domain.RoleOrganizer},
		{Label: "admin", Candidates: []string{"admin"}, Token: "cc_adm_3c5e7a9b1d0f2e46", Role: domain.RoleAdmin},
		{Label: "judge_a", Candidates: []string{"jdg_24", "judge_a"}, Token: "cc_jdg_a_91bc4d2e7a35f08", Role: domain.RoleJudge},
		{Label: "judge_b", Candidates: []string{"jdg_07", "judge_b"}, Token: "cc_jdg_b_44de8f1c6b92a07", Role: domain.RoleJudge},
		{Label: "participant", Candidates: []string{"participant"}, Token: "cc_prt_2e88a4d6c0b91f37", Role: domain.RoleParticipant},
		{Label: "participant_other", Candidates: []string{"participant_other"}, Token: "cc_prt_7b1c3f95d2e60a84", Role: domain.RoleParticipant},
	}
}

// IdentityFor returns the seeded identity that authenticates as a user id.
func IdentityFor(userID string) (Identity, bool) {
	for _, identity := range SeededIdentities() {
		for _, candidate := range identity.Candidates {
			if candidate == userID {
				return identity, true
			}
		}
	}
	return Identity{}, false
}

// TokenTable renders resolved identities as the `label = "Cookie: session=..."`
// lines consumed by the [auth] table of .dogfood.toml, sorted by label so the
// generated file is stable.
func TokenTable(identities []Resolved) []string {
	sorted := append([]Resolved(nil), identities...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Label < sorted[j].Label })
	lines := make([]string, 0, len(sorted))
	for _, identity := range sorted {
		lines = append(lines, identity.Label+" = \"Cookie: session="+identity.Token+"\"")
	}
	return lines
}

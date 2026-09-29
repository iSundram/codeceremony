import { useEffect, useState, type FormEvent } from "react";
import { Route, Routes } from "react-router-dom";

import { Alert, Badge, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { Button, Field, LinkButton } from "../../components/controls";
import { PageHead } from "../../components/shell";
import {
  Avatar,
  Card,
  CardBody,
  CardTitle,
  DescriptionList,
  DescriptionTerm,
  DescriptionValue,
  Table,
  Td,
  Th,
  Tr,
} from "../../components/data";
import { api, ApiError, type Membership, type Profile, type SessionSummary } from "../../lib/api";
import { useSession } from "../../lib/session";

/**
 * The account app.
 *
 * Everything here is the caller's own record, read from the same endpoints the
 * server-rendered pages used. There is no account switcher: a session is one
 * identity, and offering a way to flip between them would be offering a way to
 * act as someone else without a fresh sign-in.
 */
export function AccountApp() {
  const session = useSession();
  if (session.loading) return <Skeleton lines={5} />;
  if (!session.user) {
    // A signed-out visitor is sent to sign in rather than shown a shell with no
    // content in it.
    return (
      <Alert kind="info" title="Sign in to see your account">
        Your teams, your projects and your sessions are visible only to you.
      </Alert>
    );
  }
  return (
    <Routes>
      <Route index element={<Overview />} />
      <Route path="profile" element={<ProfileEditor />} />
      <Route path="teams" element={<Teams />} />
      <Route path="security" element={<Security />} />
      <Route path="*" element={<EmptyState title="No such account page" body="That address does not match an account section." icon="user" />} />
    </Routes>
  );
}

function Overview() {
  const session = useSession();
  return (
    <div className="stack stack-8">
      <PageHead
        title="My account"
        lede="Your profile, your teams, and your active sessions."
        actions={
          <LinkButton to="/account/profile" variant="secondary" icon="pencil">
            Edit profile
          </LinkButton>
        }
      />

      <div className="grid">
        <div className="col-5">
          <Card>
            <div className="cluster">
              <Avatar name={session.user?.display_name ?? ""} size="lg" />
              <div className="stack stack-1">
                <CardTitle>{session.user?.display_name}</CardTitle>
                <p className="table__meta">{session.user?.email}</p>
              </div>
            </div>
            <DescriptionList>
              <DescriptionTerm>Role</DescriptionTerm>
              <DescriptionValue>
                <Badge icon="shield">{session.user?.role}</Badge>
              </DescriptionValue>
              <DescriptionTerm>State</DescriptionTerm>
              <DescriptionValue>
                {/* design.md 10.3: a state is words and a glyph, not a colour. */}
                <Status icon="circle-check">{session.user?.state ?? "active"}</Status>
              </DescriptionValue>
            </DescriptionList>
          </Card>
        </div>
        <div className="col-7">
          <Card>
            <CardTitle>What your role can reach</CardTitle>
            <CardBody>
              These are the surfaces the portal will show you. Each one is checked against your
              role on the server for every request, so hiding a link is a convenience rather than
              the control.
            </CardBody>
            <ul className="role-list">
              {roleSurfaces(session.user?.role ?? "visitor").map((surface) => (
                <li key={surface.to} className="role-list__item">
                  <LinkButton to={surface.to} variant="tertiary" icon={surface.icon}>
                    {surface.label}
                  </LinkButton>
                </li>
              ))}
            </ul>
          </Card>
        </div>
      </div>

      <Teams />
    </div>
  );
}

function roleSurfaces(role: string): { to: string; label: string; icon: string }[] {
  if (role === "admin") {
    return [
      { to: "/admin", label: "Administration", icon: "shield" },
      { to: "/organizer", label: "Organizing", icon: "clipboard-list" },
    ];
  }
  if (role === "organizer") {
    return [
      { to: "/organizer", label: "Progress", icon: "gauge" },
      { to: "/organizer/audit", label: "Audit trail", icon: "shield" },
    ];
  }
  if (role === "judge") {
    return [
      { to: "/judge", label: "My assignments", icon: "clipboard-list" },
      { to: "/judge/compare", label: "Compare projects", icon: "git-branch" },
    ];
  }
  return [{ to: "/events", label: "Browse events", icon: "calendar" }];
}

function Teams() {
  // The teams come from the caller's own profile read, not from a listing. A
  // platform-wide teams endpoint would have the client filter it, which ships
  // every membership in the portal to the browser.
  const [teams, setTeams] = useState<Membership[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .profile()
      .then((response) => live && setTeams(response.data.memberships))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, []);

  if (error) return <ErrorState title="Your teams could not be loaded" body={error} />;
  if (!teams) return <Skeleton lines={3} />;

  return (
    <section className="stack stack-4">
      <h3 className="card__title">Your teams</h3>
      {teams.length === 0 ? (
        <EmptyState
          title="You are not on a team yet"
          body="Teams hold one to four people. An invite link is the only way in, so an organizer or an existing member has to send one."
          icon="users"
        />
      ) : (
        <Table caption="Teams you belong to, and the role you hold in each.">
          <thead>
            <tr>
              <Th>Event</Th>
              <Th>Team</Th>
              <Th>Your role</Th>
            </tr>
          </thead>
          <tbody>
            {teams.map((team) => (
              <Tr key={`${team.event_id}-${team.team_id}`}>
                <Td meta>{team.event_id}</Td>
                <Td strong>{team.team_id}</Td>
                <Td>
                  <Badge icon="users">{team.role}</Badge>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </section>
  );
}

function ProfileEditor() {
  const session = useSession();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [organization, setOrganization] = useState("");
  const [bio, setBio] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let live = true;
    api
      .profile()
      .then((response) => {
        if (!live) return;
        setProfile(response.data.profile);
        setDisplayName(response.data.profile.display_name ?? "");
        setOrganization(response.data.profile.organization ?? "");
        setBio(response.data.profile.bio ?? "");
      })
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, []);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      // The body is the account's own fields. It used to send `headline`, which
      // the server's decoder does not know — DisallowUnknownFields is on — so
      // every save came back 400 naming the offending field and the editor could
      // never be used at all.
      //
      // A headline is a UserProfile field and the account endpoint does not write
      // one, so it is not editable here. Display name is the field the user
      // actually recognises as their name on a submission.
      await api.updateProfile({ display_name: displayName, organization, bio });
      const refreshed = await api.profile();
      setProfile(refreshed.data.profile);
      setDisplayName(refreshed.data.profile.display_name ?? "");
      setOrganization(refreshed.data.profile.organization ?? "");
      setBio(refreshed.data.profile.bio ?? "");
      setSaved(true);
    } catch (caught) {
      setError(describe(caught));
    } finally {
      setBusy(false);
    }
  }

  if (error && !profile) return <ErrorState title="Your profile could not be loaded" body={error} />;
  if (!profile) return <Skeleton lines={4} />;

  return (
    <div className="stack stack-6">
      <PageHead title="Profile" lede="What other people on the panel or in the event can see about you." />

      {error ? (
        <Alert kind="error" title="That change was not saved">
          {error}
        </Alert>
      ) : null}
      {saved ? (
        <Alert kind="success" title="Profile saved">
          The change is recorded in the audit trail.
        </Alert>
      ) : null}

      <Card>
        <form className="stack stack-5" onSubmit={onSubmit}>
          <Field
            id="display_name"
            label="Display name"
            hint="The name judges and organizers see next to your reviews and submissions."
          >
            {({ id, describedBy }) => (
              <input
                id={id}
                className="input"
                aria-describedby={describedBy}
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
              />
            )}
          </Field>
          <Field id="organization" label="Organization" hint="Optional. Left blank, it is simply not shown.">
            {({ id }) => (
              <input
                id={id}
                className="input"
                value={organization}
                onChange={(event) => setOrganization(event.target.value)}
              />
            )}
          </Field>
          <Field id="bio" label="About you" hint="A sentence or two for judges who read your reviews.">
            {({ id }) => (
              <textarea
                id={id}
                className="textarea"
                rows={4}
                value={bio}
                onChange={(event) => setBio(event.target.value)}
              />
            )}
          </Field>
          <div className="actions">
            <Button variant="primary" icon="check" type="submit" loading={busy}>
              Save profile
            </Button>
            <span className="table__meta">Signed in as {session.user?.email}</span>
          </div>
        </form>
      </Card>
    </div>
  );
}

function Security() {
  const [sessions, setSessions] = useState<SessionSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .sessions()
      .then((response) => live && setSessions(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, []);

  return (
    <div className="stack stack-6">
      <PageHead title="Security" lede="Active sessions and how your account is protected." />

      <Alert kind="info" title="Sessions are not persisted across a restart">
        A restarted portal signs everyone out. That is deliberate: it means a stolen session cookie
        cannot outlive the process that issued it, at the cost of everyone signing in again after a
        deploy.
      </Alert>

      {error ? <ErrorState title="Sessions could not be loaded" body={error} /> : null}
      {sessions ? (
        <Table caption="Every session currently valid for this account.">
          <thead>
            <tr>
              <Th>Started</Th>
              <Th>Last seen</Th>
              <Th>Session</Th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((item) => (
              <Tr key={item.id}>
                <Td>{new Date(item.created_at).toLocaleString()}</Td>
                <Td>{new Date(item.last_seen_at).toLocaleString()}</Td>
                <Td>
                  {item.current ? (
                    <Status icon="circle-check">This device</Status>
                  ) : (
                    <Status icon="key" muted>
                      Other device
                    </Status>
                  )}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      ) : (
        <Skeleton lines={3} />
      )}
    </div>
  );
}

function describe(caught: unknown): string {
  return caught instanceof ApiError ? caught.message : "The portal could not be reached.";
}

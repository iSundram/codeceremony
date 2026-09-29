import { useEffect, useMemo, useRef, useState } from "react";
import { Route, Routes } from "react-router-dom";

import { Alert, Badge, EmptyState, ErrorState, Modal, Skeleton, Status } from "../../components/feedback";
import { Button, LinkButton, SearchField } from "../../components/controls";
import { PageHead, TabLink, Tabs } from "../../components/shell";
import {
  Avatar,
  Card,
  CardBody,
  CardTitle,
  InlineCode,
  StatCard,
  StatGrid,
  Table,
  Td,
  Th,
  Tr,
} from "../../components/data";
import { api, ApiError, type AuditEntry, type AuditVerification, type Grant } from "../../lib/api";
import { useSession } from "../../lib/session";

/**
 * The administration app.
 *
 * Platform-wide concerns, held by one role. It is deliberately a different
 * surface from the organizer app rather than a superset of it: an admin acting
 * across every event is a different authority from an organizer acting inside
 * one, and conflating them would make the audit trail ambiguous about which was
 * in play.
 */
export function AdminApp() {
  const session = useSession();
  if (session.loading) return <Skeleton lines={5} />;
  if (!session.user) {
    return (
      <Alert kind="info" title="Sign in to administer">
        Accounts, the platform audit trail and platform-wide grants.
      </Alert>
    );
  }
  if (session.user.role !== "admin") {
    return (
      <Alert kind="warning" title="This section is for administrators">
        Your role is {session.user.role}. The backend refuses these routes whatever the navigation
        shows.
      </Alert>
    );
  }
  return (
    <Routes>
      <Route index element={<Accounts />} />
      <Route path="audit" element={<PlatformAudit />} />
      <Route path="grants" element={<PlatformGrants />} />
      <Route
        path="*"
        element={
          <EmptyState
            title="No such administration page"
            body="That address does not match an administrative view."
            icon="shield"
          />
        }
      />
    </Routes>
  );
}

function AdminTabs({ active }: { active: string }) {
  return (
    <Tabs>
      <TabLink to="/admin" label="Accounts" active={active === "accounts"} icon="users" />
      <TabLink to="/admin/audit" label="Platform audit" active={active === "audit"} icon="shield" />
      <TabLink to="/admin/grants" label="Grants" active={active === "grants"} icon="key" />
    </Tabs>
  );
}

interface AdminUser {
  id: string;
  email: string;
  display_name: string;
  role: string;
  state: string;
  created_at: string;
}

function Accounts() {
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  useEffect(() => {
    let live = true;
    setError(null);
    void (async () => {
      try {
        const response = await fetch("/v1/admin/users", { credentials: "include" });
        if (!response.ok) throw new Error(`The portal answered ${response.status}.`);
        const body = (await response.json()) as { data: AdminUser[] };
        if (live) setUsers(body.data);
      } catch (caught) {
        if (live) setError(caught instanceof Error ? caught.message : "Accounts could not be loaded.");
      }
    })();
    return () => {
      live = false;
    };
  }, []);

  const shown = useMemo(() => {
    if (!users) return null;
    const needle = query.trim().toLowerCase();
    if (!needle) return users;
    return users.filter(
      (user) =>
        user.email.toLowerCase().includes(needle) ||
        user.display_name.toLowerCase().includes(needle) ||
        user.role.includes(needle),
    );
  }, [users, query]);

  return (
    <div className="stack stack-6">
      <PageHead
        title="Accounts"
        lede="Every account on this portal, with its role and state."
        actions={
          <LinkButton to="/admin/audit" variant="secondary" icon="shield">
            Platform audit
          </LinkButton>
        }
      />
      <AdminTabs active="accounts" />

      {error ? <ErrorState title="Accounts could not be loaded" body={error} /> : null}
      {!shown && !error ? <Skeleton lines={5} /> : null}

      {shown ? (
        <>
          <StatGrid>
            <StatCard label="Accounts" value={shown.length} icon="users" />
            <StatCard
              label="Staff"
              value={shown.filter((user) => user.role === "organizer" || user.role === "admin").length}
              meta="Organizers and admins"
              icon="shield"
            />
            <StatCard
              label="Suspended"
              value={shown.filter((user) => user.state !== "active").length}
              meta="Not able to act"
              icon="ban"
            />
          </StatGrid>

          <SearchField id="account-search" label="Search accounts" value={query} onChange={setQuery} />

          {shown.length === 0 ? (
            <EmptyState title="No accounts match" body="Nothing matches that search." icon="users" />
          ) : (
            <Table caption="Every account. Role changes take effect on the next request, because the role is re-read from the store rather than trusted from a token.">
              <thead>
                <tr>
                  <Th>Account</Th>
                  <Th>Role</Th>
                  <Th>State</Th>
                  <Th numeric>Joined</Th>
                </tr>
              </thead>
              <tbody>
                {shown.map((user) => (
                  <Tr key={user.id}>
                    <Td strong>
                      <span className="cluster cluster-2">
                        <Avatar name={user.display_name} />
                        <span className="stack stack-1">
                          {user.display_name}
                          <span className="table__meta">{user.email}</span>
                        </span>
                      </span>
                    </Td>
                    <Td>
                      <Badge icon="shield">{user.role}</Badge>
                    </Td>
                    <Td>
                      {/* design.md 10.3: a state is words and a glyph, never a
                          colour. A red row here would be exactly the colour-only
                          status the contract rules out. */}
                      {user.state === "active" ? (
                        <Status icon="circle-check">Active</Status>
                      ) : user.state === "deletion_pending" ? (
                        <Status icon="clock">Deletion pending</Status>
                      ) : (
                        <Status icon="ban">Suspended</Status>
                      )}
                    </Td>
                    <Td numeric>{new Date(user.created_at).toLocaleDateString()}</Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </>
      ) : null}
    </div>
  );
}

function PlatformAudit() {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [verification, setVerification] = useState<AuditVerification | null>(null);
  const [onlyDenied, setOnlyDenied] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    setError(null);
    void (async () => {
      try {
        const [listing, verified] = await Promise.all([api.audit({ limit: 200 }), api.verifyAudit()]);
        if (!live) return;
        setEntries(listing.data);
        setVerification(verified.data);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, []);

  const shown = useMemo(
    () => (entries ?? []).filter((entry) => (onlyDenied ? !entry.allowed : true)),
    [entries, onlyDenied],
  );

  const denied = useMemo(() => (entries ?? []).filter((entry) => !entry.allowed).length, [entries]);

  return (
    <div className="stack stack-6">
      <PageHead
        title="Platform audit"
        lede="Every authorization decision across every event, in a hash-chained log."
      />
      <AdminTabs active="audit" />

      {verification ? (
        verification.valid ? (
          <Alert kind="success" title="Chain verified">
            {`${verification.entries} entries reconcile${
              verification.dropped ? `, with ${verification.dropped} trimmed by retention` : ""
            }. Head `}
            <InlineCode>{`${verification.head.slice(0, 16)}…`}</InlineCode>
          </Alert>
        ) : (
          <Alert kind="error" title="Chain verification failed">
            {verification.detail} at sequence {verification.broken_at_seq}. This is an integrity
            failure worth investigating before anything else on this page.
          </Alert>
        )
      ) : null}

      {entries ? (
        <StatGrid>
          <StatCard label="Entries" value={entries.length} icon="shield" />
          <StatCard label="Refusals" value={denied} meta="Recorded, not dropped" icon="ban" />
        </StatGrid>
      ) : null}

      <label className="check">
        <input
          type="checkbox"
          checked={onlyDenied}
          onChange={(event) => setOnlyDenied(event.target.checked)}
        />
        <span className="check__text">Refusals only</span>
      </label>

      {error ? <ErrorState title="The audit trail could not be loaded" body={error} /> : null}
      {!entries && !error ? <Skeleton lines={5} /> : null}

      {entries && shown.length === 0 ? (
        <EmptyState
          title="No entries match"
          body={
            entries.length === 0
              ? "The log is empty. Decisions appear here as the portal is used."
              : "Nothing in the log has been refused yet. Uncheck “Refusals only” to see every decision."
          }
          icon="shield"
        />
      ) : null}

      {shown.length > 0 ? (
        <Table caption="Newest first.">
          <thead>
            <tr>
              <Th numeric>Seq</Th>
              <Th>Actor</Th>
              <Th>Action</Th>
              <Th>Outcome</Th>
              <Th>Reason</Th>
              <Th>When</Th>
            </tr>
          </thead>
          <tbody>
            {shown.map((entry) => (
              <Tr key={entry.seq}>
                <Td numeric>{entry.seq}</Td>
                <Td>{entry.actor_id || "anonymous"}</Td>
                <Td>
                  <InlineCode>{entry.action}</InlineCode>
                </Td>
                <Td>
                  {entry.allowed ? (
                    <Status icon="circle-check">Allowed</Status>
                  ) : (
                    <Status icon="ban">Refused</Status>
                  )}
                </Td>
                <Td meta>{entry.reason || "—"}</Td>
                <Td meta>{new Date(entry.created_at).toLocaleString()}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      ) : null}
    </div>
  );
}

function PlatformGrants() {
  const [grants, setGrants] = useState<Grant[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Revoking is immediate and cannot be undone, so the row is held until the
  // confirmation is answered rather than acted on the first click.
  const [revoking, setRevoking] = useState<Grant | null>(null);
  const [revokeError, setRevokeError] = useState<string | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  // A ref as well as the state: the disabled button only stops the second click
  // once React has re-rendered, and two clicks in one batch read the same value.
  const revokingRef = useRef(false);

  useEffect(() => {
    let live = true;
    setError(null);
    api
      .grants()
      .then((response) => live && setGrants(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, []);

  async function revoke() {
    if (!revoking || revokingRef.current) return;
    revokingRef.current = true;
    setRevokeBusy(true);
    setRevokeError(null);
    try {
      await api.revokeGrant(revoking.id);
      setGrants((current) => (current ?? []).filter((row) => row.id !== revoking.id));
      setRevoking(null);
    } catch (caught) {
      // Without this the rejection was unhandled: the row stayed exactly where
      // it was and nothing said why, which reads as a button that does nothing.
      setRevokeError(describe(caught));
    } finally {
      revokingRef.current = false;
      setRevokeBusy(false);
    }
  }

  return (
    <div className="stack stack-6">
      <PageHead
        title="Platform grants"
        lede="Every explicit grant and deny, across every event. Event-scoped grants are managed by the organizer who owns the event."
      />
      <AdminTabs active="grants" />

      <Card>
        <CardTitle>How resolution works</CardTitle>
        <CardBody>
          The resolver applies account state, then target constraints, then an explicit deny, then
          an explicit grant, then the event role, then the global role, then ownership. Deny before
          allow is what makes revocation immediate. Every decision carries a reason naming the rule
          that produced it, and that reason is what lands in this log.
        </CardBody>
      </Card>

      {error ? <ErrorState title="Grants could not be loaded" body={error} /> : null}
      {revokeError ? (
        <Alert kind="error" title="That grant was not revoked">
          {revokeError} The grant is still in force.
        </Alert>
      ) : null}
      {!grants && !error ? <Skeleton lines={4} /> : null}

      {grants ? (
        grants.length === 0 ? (
          <EmptyState
            title="No explicit grants"
            body="Nothing is granted or denied by hand anywhere on this portal."
            icon="key"
          />
        ) : (
          <Table caption="Every explicit grant and deny.">
            <thead>
              <tr>
                <Th>User</Th>
                <Th>Action</Th>
                <Th>Event</Th>
                <Th>Effect</Th>
                <Th>Reason</Th>
                <Th>Revoke</Th>
              </tr>
            </thead>
            <tbody>
              {grants.map((grant) => (
                <Tr key={grant.id}>
                  <Td strong>{grant.user_id}</Td>
                  <Td>
                    <InlineCode>{grant.action}</InlineCode>
                  </Td>
                  <Td meta>{grant.event_id ?? "platform"}</Td>
                  <Td>
                    {grant.allow ? (
                      <Status icon="circle-check">Allow</Status>
                    ) : (
                      <Status icon="ban">Deny</Status>
                    )}
                  </Td>
                  <Td meta>{grant.reason}</Td>
                  <Td>
                    <Button
                      variant="ghost"
                      icon="trash-2"
                      type="button"
                      onClick={() => {
                        setRevokeError(null);
                        setRevoking(grant);
                      }}
                    >
                      Revoke
                    </Button>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )
      ) : null}

      <Modal
        open={revoking !== null}
        title="Revoke this grant?"
        onClose={() => {
          if (!revokeBusy) setRevoking(null);
        }}
        closeLabel="Keep the grant"
        actions={
          <>
            <Button variant="tertiary" type="button" disabled={revokeBusy} onClick={() => setRevoking(null)}>
              Keep it
            </Button>
            <Button variant="ghost" icon="trash-2" type="button" loading={revokeBusy} onClick={() => void revoke()}>
              Revoke
            </Button>
          </>
        }
      >
        <p>
          {revoking ? (
            <>
              <InlineCode>{revoking.action}</InlineCode> for <strong>{revoking.user_id}</strong>{" "}
              {revoking.allow ? "will stop being allowed" : "will stop being denied"} immediately. It
              cannot be restored from here, and the decision is recorded in the audit trail.
            </>
          ) : null}
        </p>
      </Modal>
    </div>
  );
}

function describe(caught: unknown): string {
  return caught instanceof ApiError ? caught.message : "The portal could not be reached.";
}

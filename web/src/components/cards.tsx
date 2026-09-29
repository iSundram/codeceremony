import type { EventSummary } from "../lib/api";

import { LinkButton } from "./controls";
import { Badge, Status } from "./feedback";
import { Card, CardActions, CardBody, CardTitle } from "./data";

/**
 * The event card, in the one shape both surfaces render it.
 *
 * The same event was rendered twice with different markup: the public
 * directory used a CardTitle while the dashboard used a bare `h4` with the
 * card-title class, the two described the same date with different words, and
 * the link labels disagreed. A viewer who moved between the two met a card
 * that had changed shape for no reason.
 *
 * One definition keeps the contract clauses in one place: §8.3's card, §10.3's
 * status as words plus a glyph rather than a colour, §13's heading discipline
 * (the title is a real h3 via CardTitle, never an h4 playing at one) and a
 * machine-readable date on the closing deadline.
 */
export function EventCard({ event }: { event: EventSummary }) {
  return (
    <Card interactive>
      <div className="cluster cluster-2">
        {event.submissions_open ? (
          <Status icon="circle-check">Submissions open</Status>
        ) : (
          <Status icon="lock" muted>
            Closed
          </Status>
        )}
        <Badge icon="tag">{event.state}</Badge>
      </div>
      <CardTitle>{event.name}</CardTitle>
      {event.summary ? <CardBody>{event.summary}</CardBody> : null}
      <div className="project-card__meta">
        <span className="table__meta">
          Submissions close{" "}
          <time dateTime={event.submissions_close}>
            {new Date(event.submissions_close).toLocaleDateString()}
          </time>
        </span>
      </div>
      <CardActions>
        <LinkButton to={`/events/${event.slug}`} variant="secondary" icon="arrow-right">
          Open event
        </LinkButton>
      </CardActions>
    </Card>
  );
}

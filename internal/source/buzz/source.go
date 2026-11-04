package buzz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// Reader is the read side of the CLI the Source needs.
type Reader interface {
	Messages(ctx context.Context, channel string, since, before int64, limit int) (json.RawMessage, error)
}

const (
	// Overlap re-reads the last seconds of the previous poll: created_at has one-second
	// resolution and a message can land in a second already read. Re-read events are
	// Duplicates and cost nothing.
	Overlap = 5
	// PageSize is how many events one read asks for.
	PageSize = 100
	// maxPages bounds one poll; a backlog beyond it is read on the next poll.
	maxPages = 20
)

// ErrSecondOverflow means more than PageSize events share one second, which paging by
// time cannot get past.
var ErrSecondOverflow = errors.New("buzz: more events in one second than a page holds")

// Source polls the listened channels and hands their Triggers to the engine.
type Source struct {
	Reader   Reader
	DB       store.Querier
	Identity Identity
	Channels []string
	Now      func() time.Time
}

// Receive takes one Trigger; the engine's Receive fits it.
type Receive func(context.Context, *ticket.Trigger) error

func cursorKey(channel string) string { return "buzz:cursor:" + channel }

// Sync polls every channel once.
func (s *Source) Sync(ctx context.Context, receive Receive) error {
	var errs []error
	for _, ch := range s.Channels {
		if err := s.syncChannel(ctx, ch, receive); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", ch, err))
		}
	}
	return errors.Join(errs...)
}

// syncChannel reads a channel from its cursor and moves the cursor only after every
// Trigger was received, so a failure re-reads the same events next time (upstream
// cookbook). A channel read for the first time starts now: the history before the
// orchestrator joined is not a request to it.
func (s *Source) syncChannel(ctx context.Context, ch string, receive Receive) error {
	v, ok, err := store.GetKV(ctx, s.DB, cursorKey(ch))
	if err != nil {
		return err
	}
	if !ok {
		return store.SetKV(ctx, s.DB, cursorKey(ch), strconv.FormatInt(s.Now().Unix(), 10))
	}
	cursor, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("cursor %q: %w", v, err)
	}
	evs, err := s.read(ctx, ch, max(cursor-Overlap, 1))
	if err != nil {
		return err
	}
	Order(evs)
	next := cursor
	for _, e := range evs {
		next = max(next, e.CreatedAt)
		if e.Kind != KindChat || e.Channel() != ch {
			continue
		}
		if err := receive(ctx, s.Identity.Trigger(e)); err != nil {
			return err
		}
	}
	return store.SetKV(ctx, s.DB, cursorKey(ch), strconv.FormatInt(next, 10))
}

// read returns every event with created_at >= since. The relay answers with the newest
// page, so a full page is followed by the page before its oldest event; both bounds are
// inclusive, and events seen twice are kept once.
func (s *Source) read(ctx context.Context, ch string, since int64) ([]Event, error) {
	seen := map[string]bool{}
	var all []Event
	var before int64
	for page := 0; page < maxPages; page++ {
		raw, err := s.Reader.Messages(ctx, ch, since, before, PageSize)
		if err != nil {
			return nil, err
		}
		evs, err := ParseEvents(raw)
		if err != nil {
			return nil, err
		}
		oldest := int64(0)
		for _, e := range evs {
			if oldest == 0 || e.CreatedAt < oldest {
				oldest = e.CreatedAt
			}
			if !seen[e.ID] {
				seen[e.ID] = true
				all = append(all, e)
			}
		}
		if len(evs) < PageSize {
			return all, nil
		}
		if oldest == before {
			return nil, ErrSecondOverflow
		}
		before = oldest
	}
	return all, nil
}

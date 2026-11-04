// Command coderagent is the bhs-coderagent orchestrator: it polls the Buzz channels it
// listens to, turns explicit requests into Tickets, and posts its replies (PLAN.md F3).
//
// Its Buzz identity comes from BUZZ_PRIVATE_KEY and the relay from BUZZ_RELAY_URL; the
// buzz CLI it runs reads both from the environment.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/config"
	"github.com/boraeresici/bhs-coderagent/internal/effects"
	"github.com/boraeresici/bhs-coderagent/internal/engine"
	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/source/buzz"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/coderagent/config.yaml", "settings file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(log, *configPath); err != nil {
		log.Error("coderagent stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, configPath string) error {
	cfg, warnings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		log.Warn("config", "warning", w)
	}
	if os.Getenv("BUZZ_PRIVATE_KEY") == "" {
		return errors.New("BUZZ_PRIVATE_KEY is not set: the orchestrator has no Buzz identity")
	}
	st, err := store.Open(cfg.StateDir)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	agents := map[string]bool{cfg.Buzz.CoderPubkey: true}
	for _, pk := range cfg.Buzz.AgentPubkeys {
		agents[pk] = true
	}
	cli := &buzz.CLI{Runner: buzz.Exec{Path: cfg.Buzz.CLI}}
	q := effects.New(st.DB(), time.Now, effects.DefaultPolicies)
	eng := engine.New(st, q, time.Now, engine.Config{Self: cfg.Buzz.SelfPubkey, Channels: cfg.Buzz.Channels})
	src := &buzz.Source{
		Reader:   cli,
		DB:       st.DB(),
		Identity: buzz.Identity{Self: cfg.Buzz.SelfPubkey, Agents: agents},
		Channels: cfg.Buzz.Channels,
		Now:      time.Now,
	}
	out := &sender{w: &buzz.Writer{Client: cli, Self: cfg.Buzz.SelfPubkey}, st: st, log: log, wait: map[string]backoff{}}

	log.Info("coderagent started", "version", version, "channels", len(cfg.Buzz.Channels), "poll", cfg.Buzz.Poll.String())
	poll := time.NewTicker(cfg.Buzz.Poll)
	defer poll.Stop()
	flush := time.NewTicker(2 * time.Second)
	defer flush.Stop()
	for {
		sync(ctx, log, src, eng)
		out.flush(ctx)
		select {
		case <-ctx.Done():
			log.Info("coderagent stopping")
			return nil
		case <-poll.C:
		case <-flush.C:
			out.flush(ctx)
			continue
		}
	}
}

func sync(ctx context.Context, log *slog.Logger, src *buzz.Source, eng *engine.Engine) {
	err := src.Sync(ctx, func(ctx context.Context, tr *ticket.Trigger) error {
		r, err := eng.Receive(ctx, tr)
		if err != nil {
			return err
		}
		if r.Outcome != engine.Duplicate {
			attrs := []any{"trigger", tr.ID, "thread", tr.Threads[0].Key(), "outcome", r.Outcome}
			if r.TicketID != "" {
				attrs = append(attrs, "ticket", r.TicketID)
			}
			if r.Reason != nil {
				attrs = append(attrs, "reason", r.Reason.Code)
			}
			log.Info("trigger", attrs...)
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		log.Error("sync", "err", err)
	}
}

// sender posts the outbox in conversation order. A conversation whose head fails waits
// with a doubling delay up to five minutes; F6.1 moves this onto the effects queue.
type sender struct {
	w    *buzz.Writer
	st   *store.Store
	log  *slog.Logger
	wait map[string]backoff
}

type backoff struct {
	until time.Time
	delay time.Duration
}

func (s *sender) flush(ctx context.Context) {
	heads, err := outbox.Heads(ctx, s.st.DB())
	if err != nil {
		s.log.Error("outbox", "err", err)
		return
	}
	for _, m := range heads {
		if b, ok := s.wait[m.Conversation]; ok && time.Now().Before(b.until) {
			continue
		}
		receipt, err := s.w.Deliver(ctx, m)
		if err == nil {
			err = outbox.MarkSent(ctx, s.st.DB(), time.Now(), m.ID, receipt)
		}
		if err != nil {
			b := s.wait[m.Conversation]
			b.delay = min(max(2*b.delay, 5*time.Second), 5*time.Minute)
			b.until = time.Now().Add(b.delay)
			s.wait[m.Conversation] = b
			s.log.Error("send", "message", m.ID, "retry_in", b.delay.String(), "err", err)
			continue
		}
		delete(s.wait, m.Conversation)
		s.log.Info("sent", "message", m.ID, "event", receipt)
	}
}

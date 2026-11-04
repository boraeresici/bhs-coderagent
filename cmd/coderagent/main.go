// Command coderagent is the bhs-coderagent orchestrator: it polls the Buzz channels it
// listens to, turns explicit requests into Tickets, and posts its replies (PLAN.md F3).
//
// Its Buzz identity comes from BUZZ_PRIVATE_KEY and the relay from BUZZ_RELAY_URL; the
// buzz CLI it runs reads both from the environment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/api"
	"github.com/boraeresici/bhs-coderagent/internal/config"
	"github.com/boraeresici/bhs-coderagent/internal/effects"
	"github.com/boraeresici/bhs-coderagent/internal/engine"
	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/source/buzz"
	"github.com/boraeresici/bhs-coderagent/internal/source/github"
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
	// The MVP works in one repository (ANALIZ.md §10): the first profile.
	repo := cfg.Repos[0]
	eng := engine.New(st, q, time.Now, engine.Config{
		Self: cfg.Buzz.SelfPubkey, Channels: cfg.Buzz.Channels,
		Coder: cfg.Buzz.CoderPubkey, CoderName: cfg.Buzz.CoderName,
		Repo:       engine.Repo{GitHub: repo.GitHub, Base: repo.Base, AllowedLogins: repo.AllowedLogins},
		GitHubSelf: cfg.GitHub.SelfLogins,
	})
	src := &buzz.Source{
		Reader:   cli,
		DB:       st.DB(),
		Identity: buzz.Identity{Self: cfg.Buzz.SelfPubkey, Agents: agents},
		Channels: cfg.Buzz.Channels,
		Now:      time.Now,
	}
	// The GitHub Source is off without a gh CLI: pull requests are then followed only through
	// coder reports, and their comments, CI and merges do not reach Tickets.
	var gh *github.Source
	ghTick := make(<-chan time.Time)
	if cfg.GitHub.CLI != "" || cfg.GitHub.APIURL != "" {
		var api github.API = &github.CLI{Runner: github.Exec{Path: cfg.GitHub.CLI}}
		if cfg.GitHub.APIURL != "" {
			if cfg.GitHub.TokenEnv != "" && os.Getenv(cfg.GitHub.TokenEnv) == "" {
				return fmt.Errorf("github.token_env names %s, which is not set", cfg.GitHub.TokenEnv)
			}
			api = &github.HTTP{BaseURL: cfg.GitHub.APIURL, Token: os.Getenv(cfg.GitHub.TokenEnv)}
		}
		gh = &github.Source{API: api, DB: st.DB(), Repo: repo.GitHub, Now: time.Now}
		t := time.NewTicker(cfg.GitHub.Poll)
		defer t.Stop()
		ghTick = t.C
	}
	out := &sender{w: &buzz.Writer{Client: cli, Self: cfg.Buzz.SelfPubkey}, st: st, eng: eng, log: log}

	handler, err := api.New(st, eng, os.Getenv(cfg.API.TokenEnv))
	if err != nil {
		return fmt.Errorf("%w: set %s", err, cfg.API.TokenEnv)
	}
	srv := &http.Server{Addr: cfg.API.Listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api", "err", err)
			stop()
		}
	}()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Info("coderagent started", "api", cfg.API.Listen, "version", version, "channels", len(cfg.Buzz.Channels), "poll", cfg.Buzz.Poll.String())
	poll := time.NewTicker(cfg.Buzz.Poll)
	defer poll.Stop()
	flush := time.NewTicker(2 * time.Second)
	defer flush.Stop()
	for {
		sync(ctx, log, src, eng)
		runEffects(ctx, log, q, eng)
		out.flush(ctx)
		select {
		case <-ctx.Done():
			log.Info("coderagent stopping")
			return nil
		case <-poll.C:
		case <-ghTick:
			syncGitHub(ctx, log, gh, eng)
		case <-flush.C:
			runEffects(ctx, log, q, eng)
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

// syncGitHub binds Ticket branches' pull requests and reads the followed ones.
func syncGitHub(ctx context.Context, log *slog.Logger, src *github.Source, eng *engine.Engine) {
	err := src.Sync(ctx, eng.BindThread, func(ctx context.Context, tr *ticket.Trigger) error {
		r, err := eng.Receive(ctx, tr)
		if err != nil {
			return err
		}
		if r.Outcome != engine.Duplicate {
			attrs := []any{"trigger", tr.ID, "thread", tr.Threads[0].Key(), "event", tr.Event, "outcome", r.Outcome}
			if r.Reason != nil {
				attrs = append(attrs, "reason", r.Reason.Code)
			}
			log.Info("github", attrs...)
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		log.Error("github sync", "err", err)
	}
}

// runEffects runs the due launch and delivery effects. A LaterError puts an effect back
// without counting an attempt; any other error counts one and backs off.
func runEffects(ctx context.Context, log *slog.Logger, q *effects.Queue, eng *engine.Engine) {
	for _, kind := range []string{effects.Launch, effects.Delivery} {
		p, _ := q.Policy(kind)
		due, err := q.Claim(ctx, kind, p.Concurrency)
		if err != nil {
			log.Error("effects", "kind", kind, "err", err)
			continue
		}
		for _, ef := range due {
			var payload struct{ Ticket, Assignment string }
			err := json.Unmarshal(ef.Payload, &payload)
			if err == nil {
				switch kind {
				case effects.Launch:
					err = eng.Launch(ctx, payload.Ticket, payload.Assignment)
				case effects.Delivery:
					err = eng.Deliver(ctx, payload.Ticket)
				}
			}
			var later *engine.LaterError
			switch {
			case err == nil:
				err = q.Done(ctx, ef.ID)
			case errors.As(err, &later):
				log.Info("effect later", "kind", kind, "key", ef.Key, "reason", later.Reason)
				err = q.Later(ctx, ef.ID, later.Until)
			default:
				dead, ferr := q.Fail(ctx, ef.ID, err)
				log.Error("effect failed", "kind", kind, "key", ef.Key, "dead", dead, "err", err)
				if ferr == nil && dead {
					ferr = eng.EffectDead(ctx, kind, payload.Ticket, payload.Assignment, err)
				}
				err = ferr
			}
			if err != nil {
				log.Error("effects", "kind", kind, "key", ef.Key, "err", err)
			}
		}
	}
}

// sender posts the outbox in conversation order. A failed head waits by the outbox's
// backoff, which survives a restart; from the third failure in a row its Ticket waits on a
// person (person/delivery), and the next success clears that (F6.1). A thread that got
// outbox.RateLimit messages in the window waits (F6.2).
type sender struct {
	w   *buzz.Writer
	st  *store.Store
	eng *engine.Engine
	log *slog.Logger
}

func (s *sender) flush(ctx context.Context) {
	heads, err := outbox.Heads(ctx, s.st.DB())
	if err != nil {
		s.log.Error("outbox", "err", err)
		return
	}
	for _, m := range heads {
		if time.Now().Before(m.NextAt) {
			continue
		}
		if wait, err := outbox.RateWait(ctx, s.st.DB(), time.Now(), m.Conversation); err != nil || wait > 0 {
			if err == nil {
				s.log.Info("send later", "message", m.ID, "reason", "thread rate limit", "wait", wait.String())
				err = outbox.Later(ctx, s.st.DB(), m.ID, time.Now().Add(wait))
			}
			if err != nil {
				s.log.Error("outbox", "message", m.ID, "err", err)
			}
			continue
		}
		receipt, err := s.w.Deliver(ctx, m)
		if err == nil {
			err = outbox.MarkSent(ctx, s.st.DB(), time.Now(), m.ID, receipt)
		}
		if err != nil {
			n, ferr := outbox.Fail(ctx, s.st.DB(), time.Now(), m.ID, err)
			if ferr == nil {
				ferr = s.eng.SendFailed(ctx, m.TicketID, n, err)
			}
			s.log.Error("send", "message", m.ID, "attempts", n, "retry_in", outbox.Backoff(n).String(), "err", err)
			if ferr != nil {
				s.log.Error("outbox", "message", m.ID, "err", ferr)
			}
			continue
		}
		if m.Attempts > 0 {
			if err := s.eng.SendSucceeded(ctx, m.TicketID); err != nil {
				s.log.Error("outbox", "message", m.ID, "err", err)
			}
		}
		s.log.Info("sent", "message", m.ID, "event", receipt)
	}
}

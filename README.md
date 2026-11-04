# bhs-coderagent

> 🇹🇷 [Türkçe sürüm: **README-tr.md**](README-tr.md)

An orchestrator that runs coding work for
[buzz-hermes-stack](https://github.com/boraeresici/buzz-hermes-stack) (BHS). Someone asks
for work in Buzz with `@coderagent`; the request becomes a **Ticket**, which is handed to
the `coder` Hermes agent. The agent's report and the pull request it opens are tied back to
the same Buzz thread. When the pull request is merged, the Ticket closes.

The concept model is adapted from
[deepseek-ai/dsh-superagent-prototype](https://github.com/deepseek-ai/dsh-superagent-prototype)
(Source, Trigger, Thread, Mandate, Ticket, Assignment). The mapping is in
[UPSTREAM.md](UPSTREAM.md), the upstream license in [NOTICE](NOTICE).

## How it works

```
Buzz #coder ──@coderagent──▶ Source ──Trigger──▶ engine (gate) ──▶ Ticket ──▶ "Aldım" (thread)
                                                     │
                                                     └─▶ handoff: @coder + branch agent/<short name>
coder ──report block──▶ engine ──▶ ready+PR → human review · blocked/failed → a person
GitHub PR ──comment/review/CI──▶ engine ──▶ Input to the coder · merge/close → Ticket ends
```

- **Buzz is the only command channel.** Work starts only from a person's mention on a
  listened channel. Bot and agent messages never start work.
- **GitHub carries context only.** Only the pull requests of Ticket branches are read. A
  pull request comment reaches the coder only if its author is in the repository profile's
  `allowed_logins`; an empty list lets no one through. The rule ignores repository
  visibility and does not fall back to collaborator rights. Bot reviewers get three rounds
  per pull request; from CI, only the result and its links come through.
- **Untrusted text never becomes an instruction.** Request and comment text reaches the
  coder quoted or fenced, under an "untrusted data" header. Only a report block the coder
  writes on its own thread changes a Ticket's state.
- **Durable and safe to repeat.** State lives in one SQLite file. Every side effect is
  recorded before it runs, and each sent message carries a hidden marker, so a restart
  never sends a message twice.
- **Failure handling.** Failed sends are retried with growing backoff. After three failures
  in a row the Ticket waits on a person (`person/delivery`). A per-Ticket hop limit and a
  per-thread rate limit stop loops.

Messages the orchestrator posts in Buzz are in Turkish, as is most of the in-repo
documentation.

## Status

MVP slices 1–4 are implemented: the Buzz Source, handoff and reports, the GitHub Source,
failure handling, the panel API and the production image. 13 acceptance scenarios pass
end to end against a local relay. Next: the Tickets page in the BHS panel and the live
(VPS) deployment.

## Development

Go 1.26+.

```sh
make check        # gofmt + vet + test
make build        # bin/coderagent (this machine)
make build-arm64  # bin/coderagent-linux-arm64
```

Local Buzz relay and orchestrator (needs Docker, and the BHS repository in a sibling
directory):

```sh
scripts/dev-relay.sh init && scripts/dev-relay.sh up && scripts/dev-relay.sh seed
scripts/dev-relay.sh orchestrator                 # builds the image, starts the orchestrator
scripts/dev-relay.sh buzz human messages send ... # as human | coder | outsider
scripts/dev-relay.sh api state                    # the orchestrator API
```

Acceptance tests (`buzz-cli` surface, dependency-free Node):

```sh
node tests/uat/coderagent-mvp/kos.mjs --rapor uat/paketler/coderagent-mvp/reports/<date>
```

## Layout

| Path | Contents |
|---|---|
| `internal/ticket` | Domain model, stage table, identity keys |
| `internal/engine` | The single writer: gate, fan-out, handoff, reports, GitHub rules, panel actions |
| `internal/source/buzz` | Reading and writing Buzz (buzz CLI) |
| `internal/source/github` | Ticket pull requests (`gh api` or HTTPS) |
| `internal/store`, `effects`, `outbox` | SQLite, work queue, ordered and marked outgoing messages |
| `internal/api` | `/orchestrator/api/` (bearer token) and `/healthz` |
| `deploy/` | Production image and deployment notes |
| `uat/`, `tests/uat/` | Acceptance test scenarios, runner and reports |

## Configuration

[config.example.yaml](config.example.yaml): Buzz identities and channels, how GitHub is
read, repository profiles (base branch, `allowed_logins`). Secrets are not in the config;
they come from environment variables. Deployment: [deploy/README.md](deploy/README.md).

`ANALIZ.md §…` references in code comments point to a design document kept locally; it is
not published in this repository.

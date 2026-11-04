#!/usr/bin/env bash
# Yerel Buzz relay ortami (deploy/dev). Uretim degil; yalniz 127.0.0.1'e baglanir.
#
#   scripts/dev-relay.sh init            kimlikleri ve parolalari uretir (deploy/dev/.env)
#   scripts/dev-relay.sh up              ortami baslatir, relay saglikli olana dek bekler
#   scripts/dev-relay.sh seed            uyelikler + #coder kanali (bir kez)
#   scripts/dev-relay.sh buzz <kim> ...  buzz CLI'yi bir kimlikle calistirir
#                                        kim: human | orchestrator | coder | outsider
#   scripts/dev-relay.sh env             kanal ve pubkey'leri gosterir (gizli anahtarsiz)
#   scripts/dev-relay.sh orchestrator    ikilileri + imaji derler, config uretir, orkestratoru baslatir
#   scripts/dev-relay.sh logs            orkestratorun loglari
#   scripts/dev-relay.sh api <yol>       orkestrator API'si (or. state, tickets?filter=person)
#   scripts/dev-relay.sh down [-v]       durdurur (-v: verileri de siler)
#
# Kimlikler: human = relay owner ve test eden insan; orchestrator = bhs-coderagent;
# coder = coder Hermes profilinin kimligi (Hermes olmadan elle konusturulur);
# outsider = relay uyesi ama #coder uyesi olmayan biri (UAT negatif kontrolleri).
#
# Not: macOS bash 3.2 ile calisir; metinler bu yuzden ASCII.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEV="$ROOT/deploy/dev"
ENVF="$DEV/.env"
IMG="ghcr.io/block/buzz@sha256:510978bb0bc168cd69af55ea46a307bfc935268939f6df844623344999fde7ef"

compose() { docker compose --project-directory "$DEV" -f "$DEV/docker-compose.yml" "$@"; }

load() {
  [ -f "$ENVF" ] || { echo "once: $0 init" >&2; exit 1; }
  set -a; . "$ENVF"; set +a
}

# genkey PREFIX -> PREFIX_PUBKEY / PREFIX_SECRET satirlari
genkey() {
  local out pub sec
  out=$(docker run --rm --network none --entrypoint buzz-admin "$IMG" generate-key)
  pub=$(printf '%s\n' "$out" | awk '/^Public key:/ {print $3}')
  sec=$(printf '%s\n' "$out" | awk '/^Secret key:/ {print $3}')
  [ ${#pub} -eq 64 ] && [ ${#sec} -eq 64 ] || { echo "anahtar uretilemedi" >&2; exit 1; }
  printf '%s_PUBKEY=%s\n%s_SECRET=%s\n' "$1" "$pub" "$1" "$sec"
}

rand() { openssl rand -hex 24; }

cmd_init() {
  if [ -f "$ENVF" ]; then echo "zaten var: $ENVF (silmek icin: $0 down -v && rm $ENVF)"; return; fi
  mkdir -p "$DEV/work"
  umask 077
  {
    echo "# scripts/dev-relay.sh init — $(date -u +%FT%TZ). YEREL; commit'leme."
    echo "POSTGRES_PASSWORD=$(rand)"
    echo "REDIS_PASSWORD=$(rand)"
    echo "S3_ACCESS_KEY=dev$(openssl rand -hex 6)"
    echo "S3_SECRET_KEY=$(rand)"
    echo "GIT_HMAC=$(rand)"
    genkey RELAY | sed -n 's/^RELAY_SECRET=/RELAY_SECRET=/p'
    genkey HUMAN
    genkey ORCHESTRATOR
    genkey CODER
  } > "$ENVF"
  # relay owner = test eden insan
  echo "OWNER_PUBKEY=$(sed -n 's/^HUMAN_PUBKEY=//p' "$ENVF")" >> "$ENVF"
  echo "yazildi: $ENVF (600)"
}

cmd_up() {
  load
  compose up -d --wait
  echo "relay: http://127.0.0.1:3000 (konteyner agindan: http://relay:3000)"
}

buzz_as() {
  local who="$1"; shift
  local key
  case "$who" in
    human) key="$HUMAN_SECRET" ;;
    orchestrator) key="$ORCHESTRATOR_SECRET" ;;
    coder) key="$CODER_SECRET" ;;
    outsider) key="${OUTSIDER_SECRET:?once: $0 seed}" ;;
    *) echo "kim: human | orchestrator | coder | outsider" >&2; exit 1 ;;
  esac
  compose exec -T -e BUZZ_PRIVATE_KEY="$key" cli /b/buzz "$@"
}

# outsider: defterdeki tanim (coderagent-mvp env.kullanicilar) — relay uyesi, #coder uyesi DEGIL.
# Sonradan eklendi; eski .env'lerde yoksa uretilir. Tekrar calistirmak zararsiz.
ensure_outsider() {
  if ! grep -q '^OUTSIDER_PUBKEY=' "$ENVF"; then
    (umask 077; genkey OUTSIDER >> "$ENVF")
    load
  fi
  compose exec -T relay buzz-admin add-member --pubkey "$OUTSIDER_PUBKEY" --role member
}

cmd_seed() {
  load
  ensure_outsider
  if grep -q '^CHANNEL=' "$ENVF"; then echo "zaten seed edilmis: CHANNEL=$CHANNEL"; return; fi
  for who in ORCHESTRATOR CODER; do
    pub=$(eval echo "\$${who}_PUBKEY")
    compose exec -T relay buzz-admin add-member --pubkey "$pub" --role member
  done
  out=$(buzz_as human channels create --name coder --type stream --visibility private 2>&1) || {
    echo "kanal olusturulamadi: $out" >&2; exit 1; }
  echo "$out"
  ch=$(printf '%s' "$out" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)
  [ -n "$ch" ] || { echo "kanal UUID'si ciktida bulunamadi" >&2; exit 1; }
  for who in ORCHESTRATOR CODER; do
    pub=$(eval echo "\$${who}_PUBKEY")
    buzz_as human channels add-member --channel "$ch" --pubkey "$pub"
  done
  echo "CHANNEL=$ch" >> "$ENVF"
  echo "kanal #coder: $ch"
}

cmd_env() {
  load
  echo "CHANNEL=${CHANNEL:-<seed edilmedi>}"
  echo "HUMAN_PUBKEY=$HUMAN_PUBKEY"
  echo "ORCHESTRATOR_PUBKEY=$ORCHESTRATOR_PUBKEY"
  echo "CODER_PUBKEY=$CODER_PUBKEY"
  echo "OUTSIDER_PUBKEY=${OUTSIDER_PUBKEY:-<seed edilmedi>}"
}

# Orkestrator config'i: yalniz acik anahtarlar; gizli anahtar ortam degiskeninden gelir.
cmd_orchestrator() {
  load
  [ -n "${CHANNEL:-}" ] || { echo "once: $0 seed" >&2; exit 1; }
  "$ROOT/scripts/build-binaries.sh" >/dev/null
  cat > "$DEV/work/config.yaml" <<CFG
# scripts/dev-relay.sh orchestrator tarafindan uretildi — yerel.
state_dir: /var/lib/coderagent
api: { listen: "0.0.0.0:3082", token_env: CODERAGENT_API_TOKEN }
buzz:
  cli: /usr/local/bin/buzz
  self_pubkey: "$ORCHESTRATOR_PUBKEY"
  coder_pubkey: "$CODER_PUBKEY"
  coder_name: coder
  agent_pubkeys: ["$CODER_PUBKEY"]
  channels: ["$CHANNEL"]
  poll: 5s
repos:
  - { name: dev, github: owner/dev, base: main, allowed_logins: [dev] }
CFG
  compose --profile orchestrator up -d --build --force-recreate coderagent
}

case "${1:-}" in
  init) cmd_init ;;
  up) cmd_up ;;
  seed) cmd_seed ;;
  buzz) shift; load; buzz_as "$@" ;;
  env) cmd_env ;;
  orchestrator) cmd_orchestrator ;;
  api) shift; curl -fsS -H "Authorization: Bearer dev" "http://127.0.0.1:3082/orchestrator/api/${1:-state}"; echo ;;
  logs) load; compose --profile orchestrator logs -f --no-log-prefix coderagent ;;
  down) shift; load; compose down "$@" ;;
  *) sed -n '2,15p' "$0"; exit 1 ;;
esac

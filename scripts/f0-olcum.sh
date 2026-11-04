#!/usr/bin/env bash
# F0.1 ölçümü (PLAN.md F0.1, F0.4, F0.6; ANALIZ.md §9.1) — YALNIZ OKUMA.
#
# VPS'teki bir köprü konteynerinin içinde, o köprünün kendi Buzz kimliğiyle `buzz`
# CLI'yi çalıştırır ve çıktıyı yerelde bir dosyaya yazar. Hiçbir şey göndermez,
# düzenlemez, silmez; ekleri indirmez (yalnız ek URL'lerini listeler).
#
# Kullanım:
#   scripts/f0-olcum.sh <kanal-uuid> [kök-event-id]
#
# Ortam:
#   SSH_HOST  (varsayılan buzz-vps)      — docs/runbook.md'deki ssh kısayolu
#   BRIDGE    (varsayılan ilk bridge-*)  — okuyacak köprü konteyneri; kanalda üye olmalı
#
# Çıktı mesaj içeriği taşır: scratch dizinine yaz, repoya commit'leme.
set -euo pipefail

CHANNEL="${1:?kanal UUID gerekli: buzz channels list}"
ROOT="${2:-}"
SSH_HOST="${SSH_HOST:-buzz-vps}"
OUT="${OUT:-f0-olcum-$(date +%Y%m%d-%H%M%S).txt}"

uuid='^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
[[ "$CHANNEL" =~ $uuid ]] || { echo "kanal UUID biçiminde değil: $CHANNEL" >&2; exit 1; }
[[ -z "$ROOT" || "$ROOT" =~ ^[0-9a-f]{64}$ ]] || { echo "kök event 64 hex olmalı" >&2; exit 1; }

remote() { ssh "$SSH_HOST" "$@"; }

if [[ -z "${BRIDGE:-}" ]]; then
  BRIDGE=$(remote "docker ps --format '{{.Names}}' | grep '^bridge-' | head -1")
fi
[[ -n "$BRIDGE" ]] || { echo "köprü konteyneri bulunamadı" >&2; exit 1; }

NOW=$(date +%s)
SINCE=$((NOW - 86400))

buzz() {
  echo "### buzz $*"
  remote "docker exec $BRIDGE /opt/data/.local/bin/buzz --format json $*" 2>&1 || echo "### çıkış kodu: $?"
  echo
}

{
  echo "# F0.1 ölçümü — $(date -u +%FT%TZ) — köprü: $BRIDGE"
  echo
  echo "## A1: kimlik ve kanallar"
  buzz users get
  buzz channels list --member --limit 20
  buzz channels members --channel "$CHANNEL"

  echo "## A1: kanal mesajları — thread cevapları da geliyor mu? kind'lar?"
  buzz messages get --channel "$CHANNEL" --limit 10
  buzz messages get --channel "$CHANNEL" --limit 10 --kinds 9
  buzz messages get --channel "$CHANNEL" --since "$SINCE" --limit 20

  if [[ -n "$ROOT" ]]; then
    echo "## A1: thread"
    buzz messages thread --channel "$CHANNEL" --event "$ROOT" --limit 20
  fi

  echo "## A1: bu kimliğin mention'ları (feed)"
  buzz feed get --types mentions --limit 10

  echo "## A6: düzenlenmiş bir mesaj varsa nasıl görünüyor (40003? e-etiketi?) — yukarıdaki çıktılarda ara"
  echo "## A24: ekli mesajlarda imeta etiketi — yukarıdaki çıktılarda 'imeta' ara"
} > "$OUT"

echo "yazıldı: $OUT"
echo "Bakılacaklar: alan adları (id/pubkey/created_at/kind/tags/content), 'e' etiketlerinde root/reply"
echo "işaretleri, thread cevaplarının 'messages get' içinde olup olmadığı, 40007/46010 kind'ları,"
echo "düzenleme olaylarının biçimi, 'imeta' etiketleri."

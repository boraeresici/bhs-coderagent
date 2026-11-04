#!/usr/bin/env bash
# deploy/Dockerfile'in kopyaladigi ikilileri hazirlar (sunucuda derleme yok):
#   bin/coderagent-linux-arm64   bu repodan, CGO'suz arm64
#   bin/buzz                     buzz-hermes-stack/bin'den; oradaki SHA256SUMS ile dogrulanir
#   bin/SHA256SUMS               ikisinin ozeti; Dockerfile bunu dogrular
#
#   scripts/build-binaries.sh [BHS dizini]   (varsayilan: ../buzz-hermes-stack)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BHS="${1:-$ROOT/../buzz-hermes-stack}"
[ -f "$BHS/bin/SHA256SUMS" ] || { echo "BHS bin/SHA256SUMS yok: $BHS" >&2; exit 1; }

cd "$ROOT"
make build-arm64
(cd "$BHS/bin" && grep -E '  buzz$' SHA256SUMS | shasum -a 256 -c -) || { echo "BHS buzz ozeti tutmuyor" >&2; exit 1; }
cp "$BHS/bin/buzz" bin/buzz
(cd bin && shasum -a 256 coderagent-linux-arm64 buzz > SHA256SUMS)
echo "hazir:"; cat bin/SHA256SUMS

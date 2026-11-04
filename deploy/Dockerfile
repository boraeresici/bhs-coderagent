# syntax=docker/dockerfile:1
#
# bhs-coderagent orkestratörü — ince imaj, sunucuda derleme yok (BHS Dockerfile'ıyla aynı
# gerekçe: Coolify build önbelleğini tutamıyor, VPS'te 2.4 GB boş RAM ve swap yok).
# İkililer YERELDE hazırlanır:
#   scripts/build-binaries.sh     → bin/coderagent-linux-arm64, bin/buzz, bin/SHA256SUMS
# Sunucudaki build: taban imaj + ca-certificates + COPY + checksum.
#
# Çalışma: config /etc/coderagent/config.yaml (Coolify dosya mount'u), durum
# /var/lib/coderagent (volume). Gizli değerler ortam değişkeninden: BUZZ_PRIVATE_KEY,
# BUZZ_RELAY_URL, CODERAGENT_API_TOKEN, GH_TOKEN (github.token_env).

FROM debian:bookworm-slim

# buzz CLI'nin HTTP istemcisi sistem CA deposu olmadan kurulamıyor ("builder error",
# ANALIZ §9.1); HTTPS GitHub okuması da aynı depoyu kullanır.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY bin/coderagent-linux-arm64 bin/buzz bin/SHA256SUMS /tmp/bin/
RUN cd /tmp/bin \
 && grep -E '  (coderagent-linux-arm64|buzz)$' SHA256SUMS | sha256sum -c - \
 && install -m 0755 coderagent-linux-arm64 /usr/local/bin/coderagent \
 && install -m 0755 buzz /usr/local/bin/buzz \
 && rm -rf /tmp/bin

RUN useradd --system --uid 10001 --home-dir /var/lib/coderagent --create-home coderagent
USER coderagent
VOLUME /var/lib/coderagent
EXPOSE 3082

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD ["/usr/local/bin/coderagent", "-healthcheck", "http://127.0.0.1:3082/healthz"]

ENTRYPOINT ["/usr/local/bin/coderagent"]
CMD ["-config", "/etc/coderagent/config.yaml"]

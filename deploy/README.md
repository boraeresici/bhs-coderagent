# deploy/ — orkestratörün kurulumu

`deploy/Dockerfile` ince bir imaj üretir: Debian slim, `ca-certificates`, önceden derlenmiş iki
ikili (`coderagent`, `buzz`) ve bunların SHA256 doğrulaması. Sunucuda Go derlemesi yapılmaz
(BHS'deki gerekçeyle aynı: Coolify build önbelleğini tutamıyor, VPS'te RAM dar).

## İkilileri hazırlamak (yerelde)

```bash
scripts/build-binaries.sh            # → bin/coderagent-linux-arm64, bin/buzz, bin/SHA256SUMS
docker build --platform linux/arm64 -f deploy/Dockerfile -t coderagent:<sürüm> .
```

`buzz`, `../buzz-hermes-stack/bin`'den kopyalanır ve oradaki `SHA256SUMS` ile doğrulanır. Bu
sayede orkestratör, Hermes ajanlarıyla aynı Buzz CLI sürümünü kullanır.

## Coolify'a götürmenin iki yolu (karar bekliyor)

| Yol | Nasıl | Bedeli |
|---|---|---|
| **A. BHS gibi** | `bin/` repoya commit'lenir (`.gitignore`'dan çıkar); Coolify APPLICATION, Dockerfile `deploy/Dockerfile` | Repo büyür (~20 MB ikili); repo public ise ikililer de public |
| **B. Registry** | İmaj yerelde derlenir, `ghcr.io/<hesap>/coderagent:<sürüm>`'e push; Coolify "Docker Image" uygulaması | Push için registry token'ı; sürüm etiketi elle |

## Coolify ayarları (iki yolda da aynı)

- **Mimari:** arm64.
- **Port:** 3082. Yalnız konteyner ağına açılır; panel aynı ağdan okur. Dışarıya domain verilmez.
- **Sağlık kontrolü:** `GET /healthz`. İmajın kendi HEALTHCHECK'i de var:
  `coderagent -healthcheck http://127.0.0.1:3082/healthz`. İmajda curl yok.
- **Volume:** `/var/lib/coderagent`, uid 10001 yazar. SQLite ve dizin kilidi burada durur.
  Tek replika olmalı; kilit ikinci süreci zaten reddeder.
- **Config:** `/etc/coderagent/config.yaml` dosya mount'u. Örneği `config.example.yaml`;
  içinde gizli değer yok.
- **Ortam değişkenleri (secret):**
  - `BUZZ_PRIVATE_KEY`: orkestratörün Buzz kimliği (F1.5).
  - `BUZZ_RELAY_URL`: relay adresi (konteyner ağından).
  - `CODERAGENT_API_TOKEN`: panel ↔ orkestratör bearer token'ı (`api.token_env`).
  - `GH_TOKEN`: `github.token_env` ile. Tek repo, salt okuma yeterli: orkestratör GitHub'a yazmaz.

## Yerel deneme

`scripts/dev-relay.sh orchestrator` aynı imajı derleyip yerel relay'e bağlar
(`deploy/dev/docker-compose.yml`).

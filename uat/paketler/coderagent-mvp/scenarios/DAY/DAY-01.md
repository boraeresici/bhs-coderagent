---
paket_id: coderagent-mvp
id: DAY-01
modul: DAY
baslik: "Orkestratör yeniden başlayınca hiçbir cevap ikinci kez gönderilmez"
oncelik: P1
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: f042ca89c6ab8b48
durum: aktif
spec: tests/uat/coderagent-mvp/DAY/DAY-01.spec.mjs
---

# DAY-01 — Orkestratör yeniden başlayınca hiçbir cevap ikinci kez gönderilmez

## Ön koşullar ve hazırlık
- GIR-01 ile açılmış ve devredilmiş bir iş var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | oku | scripts/dev-relay.sh buzz human messages thread | --event <kök>; ORCHESTRATOR event sayısını not et | — | evet |
| 2 | komut | docker restart coderagent-dev-coderagent-1 | — | — | evet |
| 3 | bekle | yeniden başlama + poll | 30 sn | — | evet |
| 4 | oku | scripts/dev-relay.sh buzz human messages thread | --event <kök> | — | evet |

## Kontroller
- **E1** (exp[0] "Thread'deki "Aldım" ve devir mesajlarının sayısı değişmez") — ORCHESTRATOR event sayısı 1. adımdakiyle aynı
- **E2** (exp[1] "Yeniden başlatmadan önce yazılmış mesajlar yeni iş açmaz") — Ticket sayısı değişmedi · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
—

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- 2. adım konteyner adı `coderagent-dev-coderagent-1` (deploy/dev, proje adı coderagent-dev).
- E2: `GET state` → `open` öncesi/sonrası.
- Kod referansı (defter): internal/outbox, internal/source/buzz/source.go cursor

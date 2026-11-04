---
paket_id: coderagent-mvp
id: RAP-01
modul: RAP
baslik: "coder PR'lı \"hazır\" raporu verince iş insan incelemesini bekler"
oncelik: P1
kullanici: [coder]
yuzey: buzz-cli
otomasyon: tam
persona: coder
kaynak_hash: e95a5b72c086e458
durum: aktif
spec: tests/uat/coderagent-mvp/RAP/RAP-01.spec.mjs
---

# RAP-01 — coder PR'lı "hazır" raporu verince iş insan incelemesini bekler

## Ön koşullar ve hazırlık
- devredilmiş bir iş var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz coder messages send | --reply-to <kök> --content "PR açtım\n\n```coderagent-report\nstate: ready\nsummary: x\npr: 12\n```" | — | evet |
| 2 | bekle | poll | 10 sn | — | evet |
| 3 | oku | GET /orchestrator/api/tickets/<id> | — | — | evet |

## Kontroller
- **E1** (exp[0] "İş bitmiş sayılmaz; PR kapanana dek "insan incelemesi bekliyor" durumundadır") — stage = person/review · okuma yeri doğrulandı
- **E2** (exp[1] "İş PR #12'yi takip eder") — follows içinde github:<repo>#12, own=true · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
—

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E1/E2: Okuma yeri: `GET /orchestrator/api/tickets/<id>` (internal/api/api.go:32); Ticket kimliği ack mesajındaki `<!-- coderagent:ack:<id> -->` işaretinden (internal/engine/engine.go:282).
- Ticket person/review'da kalır; PR kapanışı F5 (GitHub Source) ile gelir.
- Kod referansı (defter): internal/engine/report.go

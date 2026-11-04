---
paket_id: coderagent-mvp
id: RAP-02
modul: RAP
baslik: "coder PR'sız \"hazır\" raporu verince iş tamamlanır"
oncelik: P1
kullanici: [coder]
yuzey: buzz-cli
otomasyon: tam
persona: coder
kaynak_hash: b3a78c6102685c1f
durum: aktif
spec: tests/uat/coderagent-mvp/RAP/RAP-02.spec.mjs
---

# RAP-02 — coder PR'sız "hazır" raporu verince iş tamamlanır

## Ön koşullar ve hazırlık
- devredilmiş bir iş var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz coder messages send | rapor bloğu state: ready | — | evet |
| 2 | bekle | poll | 10 sn | — | evet |
| 3 | oku | GET /orchestrator/api/tickets/<id> | — | — | evet |

## Kontroller
- **E1** (exp[0] "İş "tamamlandı" sonucuyla biter") — ended.outcome = completed, ended.by = executor · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
—

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E1: Okuma yeri: `GET /orchestrator/api/tickets/<id>` (internal/api/api.go:32); Ticket kimliği ack mesajındaki `<!-- coderagent:ack:<id> -->` işaretinden (internal/engine/engine.go:282). Alanlar `ended.outcome`, `ended.by` (internal/ticket/model.go:177).
- Kod referansı (defter): internal/engine/report.go

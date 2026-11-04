---
paket_id: coderagent-mvp
id: GIR-02
modul: GIR
baslik: "Mention'sız insan mesajı iş başlatmaz"
oncelik: P1
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: d1e5b4d15197628d
durum: aktif
spec: tests/uat/coderagent-mvp/GIR/GIR-02.spec.mjs
---

# GIR-02 — Mention'sız insan mesajı iş başlatmaz

## Ön koşullar ve hazırlık
- #coder kanalının üyesiyim
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human messages send | --channel $CHANNEL --content "UAT-GIR-02-<zaman> bugün deploy var mı?" | — | evet |
| 2 | bekle | poll aralığı | 15 sn | — | evet |

## Kontroller
- **E1** (exp[0] "Orkestratör cevap vermez") — mesajın thread'inde ORCHESTRATOR pubkey'li event yok
- **E2** (exp[1] "Hiçbir Ticket açılmaz") — Ticket sayısı değişmedi · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
- **N1** outsider: #coder kanalına @coderagent mention'lı istek yazmak → mesaj kanala düşmez ya da cevapsız kalır; Ticket açılmaz

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E1: thread okuması kökü de döndürür; orkestratör pubkey'li kind:9 sayılır.
- E2: "Ticket sayısı" = `GET state` → `open` öncesi/sonrası + `GET tickets` içinde bu thread'e bağlı Ticket yok. Reddin nedeni `notAddressed` (internal/engine/engine.go:235).
- Kod referansı (defter): internal/engine/engine.go gate

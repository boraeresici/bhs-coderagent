---
paket_id: coderagent-mvp
id: GIR-04
modul: GIR
baslik: "Orkestratörün dinlemediği bir kanaldaki mention iş başlatmaz"
oncelik: P2
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: 37d6fb1e33349eeb
durum: aktif
spec: tests/uat/coderagent-mvp/GIR/GIR-04.spec.mjs
---

# GIR-04 — Orkestratörün dinlemediği bir kanaldaki mention iş başlatmaz

## Ön koşullar ve hazırlık
- orkestratörün üye olduğu ama config'te dinlenmeyen ikinci bir kanal var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human channels create | --name uat-gir-04-<zaman> --type stream --visibility private + orkestratörü üye ekle | — | evet |
| 2 | komut | scripts/dev-relay.sh buzz human messages send | --channel <yeni kanal> --mention $ORCHESTRATOR_PUBKEY --content "@coderagent UAT-GIR-04 bir şey yap" | — | evet |
| 3 | bekle | poll aralığı | 15 sn | — | evet |

## Kontroller
- **E1** (exp[0] "Orkestratör cevap vermez") — yeni kanaldaki thread'de ORCHESTRATOR event'i yok
- **E2** (exp[1] "Hiçbir Ticket açılmaz") — Ticket sayısı değişmedi · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
- **N1** outsider: #coder kanalına @coderagent mention'lı istek yazmak → mesaj kanala düşmez ya da cevapsız kalır; Ticket açılmaz

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- 1. adım: kanal `channels create --type stream --visibility private` + `channels add-member`; koşu sonunda arşivlenir.
- Reddin nedeni `channelNotHeard` (internal/engine/engine.go:158).
- Kod referansı (defter): internal/engine/engine.go filter

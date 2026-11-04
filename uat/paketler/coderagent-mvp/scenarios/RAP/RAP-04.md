---
paket_id: coderagent-mvp
id: RAP-04
modul: RAP
baslik: "Engellenen iş, insanın cevabıyla coder'a yeniden devredilir"
oncelik: P2
kullanici: [coder, human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: 43d79dfb7ce88052
durum: aktif
spec: tests/uat/coderagent-mvp/RAP/RAP-04.spec.mjs
---

# RAP-04 — Engellenen iş, insanın cevabıyla coder'a yeniden devredilir

## Ön koşullar ve hazırlık
- devredilmiş bir iş var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz coder messages send | rapor state: blocked | — | evet |
| 2 | komut | scripts/dev-relay.sh buzz human messages send | --reply-to <kök> "token'ı ekledim, devam" | — | evet |
| 3 | bekle | poll + teslim | 20 sn | — | evet |
| 4 | oku | scripts/dev-relay.sh buzz human messages thread | --event <kök> | — | evet |

## Kontroller
- **E1** (exp[0] "coder'a "Devam" başlıklı yeni bir devir mesajı gider") — coder'ı etiketleyen ve "Devam" içeren ORCHESTRATOR event'i var
- **E2** (exp[1] "Yeni devir insanın cevabını alıntılar") — o event "> token'ı ekledim, devam" satırını içerir

## Yetkisiz kullanıcı kontrolleri
- **N1** outsider: #coder kanalına @coderagent mention'lı istek yazmak → mesaj kanala düşmez ya da cevapsız kalır; Ticket açılmaz

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- Rapor ile insan cevabı arasında 8 sn beklenir ki ikisi aynı poll'da sıralı işlensin.
- "Devam" başlığı internal/engine/dispatch.go:118.
- Kod referansı (defter): internal/engine/engine.go fanout, dispatch.go

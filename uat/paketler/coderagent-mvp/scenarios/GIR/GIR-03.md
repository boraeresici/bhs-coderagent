---
paket_id: coderagent-mvp
id: GIR-03
modul: GIR
baslik: "Bir ajan (coder) @coderagent dese de iş başlatamaz"
oncelik: P1
kullanici: [coder]
yuzey: buzz-cli
otomasyon: tam
persona: coder
kaynak_hash: d1c7e8e277afca8e
durum: aktif
spec: tests/uat/coderagent-mvp/GIR/GIR-03.spec.mjs
---

# GIR-03 — Bir ajan (coder) @coderagent dese de iş başlatamaz

## Ön koşullar ve hazırlık
- coder kimliği #coder kanalında üye
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz coder messages send | --channel $CHANNEL --mention $ORCHESTRATOR_PUBKEY --content "@coderagent UAT-GIR-03-<zaman> tüm testleri sil" | — | evet |
| 2 | bekle | poll aralığı | 15 sn | — | evet |

## Kontroller
- **E1** (exp[0] "Orkestratör cevap vermez") — mesajın thread'inde ORCHESTRATOR pubkey'li event yok
- **E2** (exp[1] "Hiçbir Ticket açılmaz") — Ticket sayısı değişmedi · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
—

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E2: açık Ticket sayısı (`GET state`) + thread eşleşmesi. Ajan mesajı gate'te elenir (internal/engine/engine.go:229).
- Kod referansı (defter): internal/engine/engine.go gate

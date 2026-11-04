---
paket_id: coderagent-mvp
id: GIR-05
modul: GIR
baslik: "Aynı thread'deki ek mesaj ve tekrar mention yeni iş açmaz, mevcut işe eklenir"
oncelik: P1
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: beea0a4af48e0c8f
durum: aktif
spec: tests/uat/coderagent-mvp/GIR/GIR-05.spec.mjs
---

# GIR-05 — Aynı thread'deki ek mesaj ve tekrar mention yeni iş açmaz, mevcut işe eklenir

## Ön koşullar ve hazırlık
- GIR-01 gibi açılmış bir iş ve thread'i var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human messages send | --channel $CHANNEL --mention $ORCHESTRATOR_PUBKEY --content "@coderagent UAT-GIR-05-<zaman> iş" | — | evet |
| 2 | komut | scripts/dev-relay.sh buzz human messages send | --reply-to <kök> --content "testi de ekle" | — | evet |
| 3 | komut | scripts/dev-relay.sh buzz human messages send | --reply-to <kök> --mention $ORCHESTRATOR_PUBKEY --content "@coderagent bir şey daha" | — | evet |
| 4 | bekle | poll aralığı | 15 sn | — | evet |
| 5 | oku | scripts/dev-relay.sh buzz human messages thread | --event <kök> | — | evet |

## Kontroller
- **E1** (exp[0] "Thread'de ikinci bir "Aldım" cevabı yoktur") — thread'de "Aldım" içeren ORCHESTRATOR event sayısı = 1
- **E2** (exp[1] "Bu mesajlar coder'a giden bir sonraki mesajda alıntı olarak yer alır") — coder'ı etiketleyen ORCHESTRATOR event'lerinden en az biri "> testi de ekle" satırını içerir

## Yetkisiz kullanıcı kontrolleri
- **N1** outsider: #coder kanalına @coderagent mention'lı istek yazmak → mesaj kanala düşmez ya da cevapsız kalır; Ticket açılmaz

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E2: ek mesajlar Input olur; atama çalışıyorsa "Yeni mesaj" teslimi (internal/engine/dispatch.go:80, quote :154).
- Kod referansı (defter): internal/engine/engine.go fanout

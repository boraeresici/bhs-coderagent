---
paket_id: coderagent-mvp
id: TES-03
modul: TES
baslik: "coder bir işte çalışırken ikinci iş sıraya girer, ilki rapor verince devredilir"
oncelik: P2
kullanici: [human, coder]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: 6e331352aaa57df5
durum: aktif
spec: tests/uat/coderagent-mvp/TES/TES-03.spec.mjs
---

# TES-03 — coder bir işte çalışırken ikinci iş sıraya girer, ilki rapor verince devredilir

## Ön koşullar ve hazırlık
- coder boşta
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human messages send | iki ayrı kök mesaj, ikisi de @coderagent | — | evet |
| 2 | bekle | poll + teslim | 20 sn | — | evet |
| 3 | oku | scripts/dev-relay.sh buzz human messages thread | iki thread de | — | evet |
| 4 | komut | scripts/dev-relay.sh buzz coder messages send | --reply-to <1. kök> rapor bloğu state: ready | — | evet |
| 5 | bekle | busy bekleme | 45 sn | — | evet |
| 6 | oku | scripts/dev-relay.sh buzz human messages thread | 2. thread | — | evet |

## Kontroller
- **E1** (exp[0] "Aynı anda coder'a yalnız bir iş devredilir") — 3. adımda coder'ı etiketleyen assignment yalnız 1. thread'de
- **E2** (exp[1] "İlk iş rapor verdikten sonra ikinci iş devredilir") — 6. adımda 2. thread'de de assignment var

## Yetkisiz kullanıcı kontrolleri
—

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- coder meşgulken Launch 30 sn sonra yeniden dener (busyWait, internal/engine/dispatch.go:25); 45 sn bekleme bunu kapsar.
- Kod referansı (defter): internal/engine/dispatch.go coderBusy

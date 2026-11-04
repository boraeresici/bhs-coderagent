---
paket_id: coderagent-mvp
id: TES-01
modul: TES
baslik: "İş coder'a kendi kendine yeterli bir mesajla devredilir"
oncelik: P1
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: acfdd30b47a128d2
durum: aktif
spec: tests/uat/coderagent-mvp/TES/TES-01.spec.mjs
---

# TES-01 — İş coder'a kendi kendine yeterli bir mesajla devredilir

## Ön koşullar ve hazırlık
- coder başka bir işte çalışmıyor
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human messages send | --mention $ORCHESTRATOR_PUBKEY --content "@coderagent UAT-TES-01-<zaman> login hatası" | — | evet |
| 2 | bekle | poll + teslim | 20 sn | — | evet |
| 3 | oku | scripts/dev-relay.sh buzz human messages thread | --event <kök> | — | evet |

## Kontroller
- **E1** (exp[0] "Orkestratörün coder'ı etiketleyen (mention) bir mesajı vardır") — ORCHESTRATOR event'inde ["p", $CODER_PUBKEY] etiketi var
- **E2** (exp[1] "Mesajda Ticket kimliği, `agent/<kısa ad>` dalı, taban dal ve repo yazar") — içerik "agent/", "taban:" ve config'teki repo adını içerir
- **E3** (exp[2] "İstek metni alıntı olarak ve "güvenilmeyen veri" uyarısıyla yer alır") — istek satırı "> " ile başlar; "güvenilmeyen veri" ifadesi geçer
- **E4** (exp[3] "Mesaj coder'dan rapor bloğu ister") — içerik "```coderagent-report" bloğu şablonunu içerir

## Yetkisiz kullanıcı kontrolleri
—

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E2: repo adı yerel config'te `owner/dev` (scripts/dev-relay.sh orchestrator).
- Devir metni internal/engine/dispatch.go:116-125.
- Kod referansı (defter): internal/engine/dispatch.go Launch

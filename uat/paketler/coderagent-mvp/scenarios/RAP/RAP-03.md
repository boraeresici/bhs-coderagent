---
paket_id: coderagent-mvp
id: RAP-03
modul: RAP
baslik: "İnsanın yazdığı rapor bloğu işin durumunu değiştirmez"
oncelik: P1
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: b69439d5b4d7ef3e
durum: aktif
spec: tests/uat/coderagent-mvp/RAP/RAP-03.spec.mjs
---

# RAP-03 — İnsanın yazdığı rapor bloğu işin durumunu değiştirmez

## Ön koşullar ve hazırlık
- devredilmiş, açık bir iş var
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human messages send | --reply-to <kök> rapor bloğu state: ready | — | evet |
| 2 | bekle | poll + teslim | 15 sn | — | evet |
| 3 | oku | scripts/dev-relay.sh buzz human messages thread | --event <kök> | — | evet |

## Kontroller
- **E1** (exp[0] "İşin durumu değişmez; blok coder'a ek mesaj olarak iletilir") — Ticket stage değişmedi; coder'ı etiketleyen yeni bir "yeni mesaj" teslimi var ve bloğu "> " ile alıntılar · okuma yeri doğrulandı

## Yetkisiz kullanıcı kontrolleri
- **N1** outsider: #coder kanalına @coderagent mention'lı istek yazmak → mesaj kanala düşmez ya da cevapsız kalır; Ticket açılmaz

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E1: stage öncesi/sonrası Okuma yeri: `GET /orchestrator/api/tickets/<id>` (internal/api/api.go:32); Ticket kimliği ack mesajındaki `<!-- coderagent:ack:<id> -->` işaretinden (internal/engine/engine.go:282). Teslim "> " alıntısı internal/engine/dispatch.go:154.
- Kod referansı (defter): internal/engine/engine.go fanout, report.go

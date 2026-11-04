---
paket_id: coderagent-mvp
id: GIR-01
modul: GIR
baslik: "Kanal üyesi bir insan @coderagent der, aynı thread'de iş alındı cevabı gelir"
oncelik: P1
kullanici: [human]
yuzey: buzz-cli
otomasyon: tam
persona: human
kaynak_hash: b3e3aa1e6ff4925e
durum: aktif
spec: tests/uat/coderagent-mvp/GIR/GIR-01.spec.mjs
---

# GIR-01 — Kanal üyesi bir insan @coderagent der, aynı thread'de iş alındı cevabı gelir

## Ön koşullar ve hazırlık
- #coder kanalının üyesiyim
- orkestratör #coder kanalını dinliyor
- (komut) `scripts/dev-relay.sh up && scripts/dev-relay.sh seed && make build-arm64 && scripts/dev-relay.sh orchestrator` — yerel relay, #coder kanalı ve orkestratör ayakta

## Adımlar
| # | Eylem | Hedef | Veri | Kontrol | Doğrulandı mı |
|---|---|---|---|---|---|
| 1 | komut | scripts/dev-relay.sh buzz human messages send | --channel $CHANNEL --mention $ORCHESTRATOR_PUBKEY --content "@coderagent UAT-GIR-01-<zaman> README'ye kurulum bölümü ekle" | {"accepted":true} | evet |
| 2 | bekle | poll aralığı | 15 sn | — | evet |
| 3 | oku | scripts/dev-relay.sh buzz human messages thread | --channel $CHANNEL --event <1. adımın event_id> | — | evet |

## Kontroller
- **E1** (exp[0] "Thread'de orkestratörden tek bir "Aldım" cevabı vardır") — thread'de pubkey=ORCHESTRATOR olan kind:9 event sayısı = 1
- **E2** (exp[1] "Cevap Ticket'ın kısa adını (dal adı olarak kullanılacak ad) içerir") — o event'in içeriğinde `<slug>-<6 hex>` biçiminde bir kısa ad var · okuma yeri doğrulandı
- **E3** (exp[2] "Kanalın ana akışına ayrı bir mesaj düşmez; cevap thread içindedir") — orkestratör event'inde ["e", <kök>] etiketi var (thread cevabı)

## Yetkisiz kullanıcı kontrolleri
- **N1** outsider: #coder kanalına @coderagent mention'lı istek yazmak → mesaj kanala düşmez ya da cevapsız kalır; Ticket açılmaz

## Görsel, erişilebilirlik, 390px
Yüzey buzz-cli: ekran, a11y ve mobil kontrol noktası yok.

## Açık sorular
—

## Doğrulama notları
- E1: exp[0] "tek bir Aldım cevabı". uat.kontrol tüm orkestratör event'lerini sayıyor; devir mesajı (TES-01) da orkestratörden aynı thread'e gelir. Spec "Aldım" içerenleri sayar, ham sayıyı not eder → yama önerisi.
- E2: kısa ad ack metninde backtick içinde (internal/engine/engine.go:280), biçim internal/engine/names.go.
- E3: thread yanıtı etiketi `["e",<kök>,"","reply"]` (testdata/send_reply.json).
- Kod referansı (defter): internal/engine/engine.go (gate, start)

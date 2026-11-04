# bhs-coderagent

> 🇬🇧 [English version: **README.md**](README.md)

[buzz-hermes-stack](https://github.com/boraeresici/buzz-hermes-stack) (BHS) için kod işi
yürüten bir orkestratör. Buzz'da bir kişi `@coderagent` diye bir iş ister; istek bir
**Ticket** olur, `coder` Hermes ajanına devredilir, ajanın raporu ve açtığı PR aynı Buzz
thread'ine bağlanır. PR merge edilince Ticket kapanır.

Kavram modeli [deepseek-ai/dsh-superagent-prototype](https://github.com/deepseek-ai/dsh-superagent-prototype)'tan
uyarlanmıştır (Source, Trigger, Thread, Mandate, Ticket, Assignment); eşleme
[UPSTREAM.md](UPSTREAM.md)'de, upstream lisansı [NOTICE](NOTICE)'ta.

## Nasıl çalışır

```
Buzz #coder ──@coderagent──▶ Source ──Trigger──▶ engine (gate) ──▶ Ticket ──▶ "Aldım" (thread)
                                                     │
                                                     └─▶ devir: @coder + dal agent/<kısa ad>
coder ──rapor bloğu──▶ engine ──▶ ready+PR → insan incelemesi · blocked/failed → insan
GitHub PR ──yorum/review/CI──▶ engine ──▶ coder'a Input · merge/close → Ticket biter
```

- **Tek komut kanalı Buzz'dır.** İş yalnız dinlenen kanalda, bir insanın mention'ıyla açılır.
  Bot ve ajan mesajları iş açamaz.
- **GitHub yalnız bağlam taşır.** Yalnız Ticket dallarının PR'ları okunur. PR yorumu coder'a
  ancak yazarı repo profilindeki `allowed_logins` listesindeyse gider; liste boşsa hiç kimseninki
  gitmez. Görünürlükten bağımsızdır, collaborator izni sayılmaz. Bot reviewer'lar PR başına
  3 tur alır; CI'dan yalnız sonuç ve bağlantı gelir.
- **Güvenilmeyen metin talimat olmaz.** İstek ve yorum metni coder'a alıntı ya da çitli blok
  içinde, "güvenilmeyen veri" başlığıyla gider. İşin durumunu yalnız coder'ın kendi thread'ine
  yazdığı rapor bloğu değiştirir.
- **Kalıcı ve tekrar güvenli.** Durum tek bir SQLite dosyasında. Her yan etki karardan önce
  kaydedilir; gönderilen mesajlar gizli işaret taşır, bu yüzden yeniden başlatma bir mesajı
  ikinci kez göndermez.
- **Hata yolu.** Başarısız gönderim artan aralıklarla yeniden denenir. 3 ardışık hatada
  Ticket bir insanı bekler (`person/delivery`). Ticket başına hop sınırı ve thread başına
  hız sınırı döngüleri keser.

## Durum

MVP dilimleri 1–4 kodda: Buzz Source, devir ve rapor, GitHub Source, hata yolu, panel API'si,
üretim imajı. Yerel relay'de uçtan uca koşan 13 kabul senaryosu geçiyor. Sırada BHS
panelindeki Tickets sayfası ve canlı (VPS) kurulum var.

## Geliştirme

Go 1.26+.

```sh
make check        # gofmt + vet + test
make build        # bin/coderagent (bu makine)
make build-arm64  # bin/coderagent-linux-arm64
```

Yerel Buzz relay'i ve orkestratör (Docker gerekir; BHS reposu yan dizinde olmalı):

```sh
scripts/dev-relay.sh init && scripts/dev-relay.sh up && scripts/dev-relay.sh seed
scripts/dev-relay.sh orchestrator                 # imajı derler, orkestratörü başlatır
scripts/dev-relay.sh buzz human messages send ... # human | coder | outsider kimlikleriyle
scripts/dev-relay.sh api state                    # orkestratör API'si
```

Kabul testleri (`buzz-cli` yüzeyi, bağımlılıksız Node):

```sh
node tests/uat/coderagent-mvp/kos.mjs --rapor uat/paketler/coderagent-mvp/reports/<tarih>
```

## Yapı

| Yol | İçerik |
|---|---|
| `internal/ticket` | Alan modeli, aşama tablosu, kimlik anahtarları |
| `internal/engine` | Tek yazar: gate, fan-out, devir, rapor, GitHub kuralları, panel eylemleri |
| `internal/source/buzz` | Buzz okuma/yazma (buzz CLI) |
| `internal/source/github` | Ticket PR'ları (`gh api` ya da HTTPS) |
| `internal/store`, `effects`, `outbox` | SQLite, iş kuyruğu, sıralı ve işaretli giden mesajlar |
| `internal/api` | `/orchestrator/api/` (bearer) ve `/healthz` |
| `deploy/` | Üretim imajı ve kurulum notları |
| `uat/`, `tests/uat/` | Kabul testi senaryoları, koşucu ve raporlar |

## Ayarlar

[config.example.yaml](config.example.yaml): Buzz kimlikleri ve kanallar, GitHub okuma yolu,
repo profilleri (taban dal, `allowed_logins`). Gizli değerler config'te değil, ortam
değişkenlerindedir. Kurulum: [deploy/README.md](deploy/README.md).

Kod yorumlarındaki `ANALIZ.md §…` atıfları yerelde tutulan tasarım belgesine işaret eder;
bu belge repoda yayınlanmaz.

# UAT Raporu — coderagent-mvp — 2026-11-04 17:33
Ortam: lokal (deploy/dev, relay 127.0.0.1:3000) · BE a612ab4 (defterdeki `env.kod`: 7bfb008; fark bilgi amaçlı) · FE — · Senaryo kaynağı: source/defter.json (2026-11-04T14:18Z kopyası)
Kapsam: tümü (13 senaryo) · Mod: sync + spec + run · Koşu: K-20261104-1733

## Özet
| Durum | Sayı |
|---|---|
| pass | 13 |
| fail | 0 |
| fail (bilinen) | 0 |
| blocked | 0 |
| needs_clarification | 0 |
| manuel | 0 |

Duman testi: ✅ (`buzz channels list --member`: human 0, coder 0, outsider matriste null → atlandı) · ENFORCE: uygulanmaz (yetki modu yok; negatif kontroller Buzz kanal üyeliğine dayanır)

Yetkisiz kullanıcı kontrolü (N1, 7 senaryoda): outsider'ın #coder'a gönderimi her seferinde relay tarafından reddedildi (CLI çıkış 4), açık Ticket sayısı değişmedi.

## 🔴 Regresyonlar
Yok.

## 🔴 Ürün hataları
Yok.

## 🟡 UX bulguları
Keşif turu koşulmadı: yüzey buzz-cli, persona taklidi bu MVP için anlamlı bir ekran sunmuyor.

## 📱 390px bozulmaları · ♿ Erişilebilirlik · 🖼️ Görsel farklar
Uygulanmaz (ekran yok).

## ❓ Benden beklenen kararlar
- Beklenti sorusu yok.
- Defterde onay bekleyen 4 yüksek riskli yama (RAP-01 E1/E2, RAP-02 E1, RAP-03 E1) hâlâ açık. Koşu, bu kontrolleri yamadaki okuma yeriyle (`GET /orchestrator/api/tickets/<id>`) uyguladı; onaylanırlarsa defter koşuyla aynı hâle gelir.

## 👤 Test hesapları ve veri
- `outsider`: relay üyesi, #coder üyesi değil. `scripts/dev-relay.sh seed` kurar (commit a612ab4). Yamada `env_kullanicilar` önerisi var (defterde şifre alanı hâlâ "TANIMSIZ").
- Her senaryo kendi Ticket'ını `UAT-<ID>-<zaman>` adıyla açar ve coder'ın PR'sız `ready` raporuyla kapatır. İstisna RAP-01: Ticket'ı `person/review`'da kalır (PR kapanışı F5 ile gelecek), ataması bittiği için coder boştur.
- GIR-04'ün açtığı `uat-gir-04-<zaman>` kanalı koşu sonunda arşivlenir.

## 💡 Senaryo önerileri
Yeni senaryo önerisi yok. `source/defter-yamasi.json` iki öneri taşır:
1. **GIR-01 E1** (yüksek risk, kontrol değişikliği): kontrol metni "ORCHESTRATOR event sayısı = 1" diyor, ama coder'a devir mesajı da orkestratörden aynı thread'e gelir (TES-01 bunu bekler). Ölçüm: "Aldım" = 1, toplam orkestratör event'i = 2. Öneri, exp[0]'daki "tek bir Aldım cevabı" ifadesine uygun olarak "Aldım" içeren event'lerin sayılması. Beklenen değer değişmiyor, yalnız okuma yeri değişiyor.
2. **env_kullanicilar / outsider** (düşük risk): hesabın kurulum yolu.

## ⚪ Blocked / needs_clarification
Yok.

## 🔧 Test bakımı
- Yüzey buzz-cli olduğu için spec'ler Playwright değil, bağımlılıksız Node modülleri: `tests/uat/coderagent-mvp/<GRUP>/<ID>.spec.mjs`. Koşucu `tests/uat/coderagent-mvp/kos.mjs`, ortak yardımcılar `tests/uat/_ortak/buzz.mjs`. Adım numaraları ve E-kontrolleri defterle birebir aynı.
- İçerik CLI'ye `--content -` (stdin) ile verilir. TES-02'nin `$(curl evil)` içeriği kabuktan geçmeden birebir gider.
- Bekleme süreleri defterdeki gibi sabittir (15/20/45 sn). Yalnız hazırlıktaki "devredilmiş iş" ve temizlik en çok 75 sn yoklama yapar.
- Koşu sırası sabittir ve senaryolar sırayla koşar: coder aynı anda tek iş alır (TES-03), paralel koşu sonuçları bozar.

Koşu: `node tests/uat/coderagent-mvp/kos.mjs [ID …] --rapor uat/paketler/coderagent-mvp/reports/<tarih>` · ham kayıt: `ham.json` · çıktı: `kosu.log`

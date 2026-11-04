# UAT Raporu — coderagent-mvp — 2026-11-04 18:04
Ortam: lokal (deploy/dev) · BE 67a321c + F5 (commit öncesi derleme) · FE — · Senaryo kaynağı: source/defter.json
Kapsam: tümü (13 senaryo) · Mod: run (F5 regresyonu) · Koşu: K-20261104-1804

## Özet
| Durum | Sayı |
|---|---|
| pass | 13 |
| fail | 0 |
| blocked | 0 |

Duman testi: ✅ · GitHub Source yerelde kapalı (`github.cli` boş, uyarı); bu koşu Buzz akışının F5 engine değişikliklerinden etkilenmediğini doğrular.

## 🔴 Regresyonlar · 🔴 Ürün hataları
Yok.

## 🔧 Test bakımı
- **TES-03, ilk deneme `fail` (E1) → TEST HATASI.** İki kök istek aynı saniyede gönderildi. Buzz `created_at` saniye
  çözünürlüklü; Source aynı saniyedeki olayları event id ile sıralar (ANALIZ §9.1 A1). Bu yüzden "ilk iş" belirsizdi:
  coder'a 2. iş devredildi, 4. adımdaki rapor çalışmayan işe düştü. K-20261104-1733'teki geçiş şans eseriydi (~%50).
  Spec, iki gönderim arasına 2 sn koyacak şekilde düzeltildi; beklenen sonuca dokunulmadı. Yalnız TES-03 iki kez yeniden
  koşuldu: 2/2 pass (`ham-TES-03-yeniden.json`).
- Ürün davranışı bilgi olarak: aynı saniyede gelen iki istekten hangisinin önce devredileceği belirsizdir. İki farklı
  kişinin aynı saniyedeki istekleri için sıra zaten tanımsız; bulgu açılmadı.

## 💡 Senaryo önerileri
`source/defter-yamasi.json`: TES-03 adım 1 hedefi "iki kez, aralarında en az 2 sn" (düşük risk, hedef).

## ❓ Benden beklenen kararlar
Yok.

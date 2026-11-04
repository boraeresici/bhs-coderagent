# UAT Raporu — coderagent-mvp — 2026-11-04-2123
Ortam: lokal · orkestratör **üretim imajıyla** (deploy/Dockerfile, uid 10001, ca-certificates gömülü, yeni volume) · BE 3192b76 + F6.5 · Koşu: K-20261104-2123

## Özet
| Durum | Sayı |
|---|---|
| pass | 13 |
| fail / blocked | 0 |

Duman testi ✅ · Docker HEALTHCHECK DAY-01 yeniden başlatmasından sonra da `healthy`.
Bulgu yok. Bu koşu F6.1–F6.5 değişikliklerinin (kalıcı backoff, devre kesici, hız sınırı, retry/stop/pause, lease bırakma, /healthz, imaj) Buzz akışını bozmadığını doğrular.

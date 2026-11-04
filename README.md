# bhs-coderagent

buzz-hermes-stack (BHS) için kod yazan, proaktif bir orkestratör. Buzz'da izin verilen bir
insanın isteği bir **Ticket** olur; iş `coder` Hermes profiline delege edilir, sonuç aynı
Buzz thread'ine ve GitHub PR'ına döner.

- Tasarım ve kararlar: [ANALIZ.md](ANALIZ.md)
- Faz / iş paketi planı ve durum: [PLAN.md](PLAN.md)
- Upstream eşleme: [UPSTREAM.md](UPSTREAM.md)

## Geliştirme

Go 1.26+.

```sh
make check        # gofmt + vet + test
make build        # bin/coderagent
make build-arm64  # VPS için linux/arm64
```

Durum: F2 (saf çekirdek) bitti; F3 (Buzz Source) F0 ölçümlerini bekliyor. Servis döngüsü henüz yok.

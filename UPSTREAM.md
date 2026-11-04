# Upstream takibi

**Upstream:** [deepseek-ai/dsh-superagent-prototype](https://github.com/deepseek-ai/dsh-superagent-prototype)
**İzlenen commit:** `8c32a9dc5d72c033816c6a8110b37f40cf94fba4` (2026-10-31)
**Lisans:** MIT — metin [NOTICE](NOTICE)'ta.

Upstream kavram düzeyinde izlenir, kod satır satır taşınmaz (ANALIZ.md §0 karar 6, §7).
Terimler aynı tutulur: Source, Trigger, Thread, Channel, Policy, Eval, Mandate, Ticket,
Assignment, Report, Input.

## Eşleme

| Upstream | Bizde | Durum | Not |
|---|---|---|---|
| `src/core/ids.ts` `key()` | `internal/ticket/ids.go` `Key()` | taşındı (birebir) | Çıktı upstream ile aynı; `ids_test.go` Node'dan üretilmiş vektörlerle doğrular |
| `src/core/stage.ts` `ticketStage`, `canRetry` | `internal/ticket/stage.go` `StageOf`, `CanRetry` | taşındı (birebir) | Tablo sırası aynı, "ilk eşleşen kazanır" |
| `src/core/model.ts` | `internal/ticket/model.go` | kısmi | Aşağıdaki "Alınmayan alanlar" |
| `src/core/reasons.ts` | `internal/ticket/reasons.go` | kısmi | Yalnız MVP gate'inin kullandığı kodlar + bizim kodlarımız |
| `src/app/store.ts` | `internal/store` | yapılmadı (F2.6) | |
| `src/app/effects.ts` | `internal/effects` | yapılmadı (F2.7) | |
| `src/sources/*` | `internal/source/{buzz,github}` | yapılmadı (F3, F5) | Buzz Source upstream'de yok — yerel |
| Red team kapıları, bulgu döngüsü | — | **karşılığı yok — yerel ekleme** | ANALIZ §5.11 |
| QA / kabul testi | — | **karşılığı yok — yerel ekleme** | ANALIZ §5.12 |

## `model.ts`'ten alınmayanlar (bilinçli)

MVP'de Eval yok (gate deterministik, ANALIZ §10), tek Source ailesi ve tek repo var. Bu
yüzden şunlar şimdilik alınmadı; ihtiyaç doğduğunda upstream tanımından eklenir:

- `TriggerDisposition` ayrıntıları (`handbacks`, `acked`, `requeued`, `context`) ve `followupOwners`
- `Evaluation`, `Decision` gövdesi, `AcceptMode`, `AdmissionCell`
- `MandateBasis` birleşik tipi → tek yapı (`Via`, `Addressed`, `Scope`, `Sender`, `Rules`)
- `TicketInput` türlerinden `policy-changed`, `reopen` (yalnız `trigger`, `resumed`, `text`, `note`)
- `Assignment.idleAt`, `Ticket.answers`, `readUntil`, `reached`
- Zaman alanları ISO string değil `time.Time` (upstream string karşılaştırması yapıyor; anlam aynı)

## Takip prosedürü

ANALIZ.md §7. Yeni upstream commit'inde: `src/core/*`, `docs/00*.md`, `prompts/*` diff'i
okunur; etkilenen satır bu tabloda güncellenir, izlenen commit değişir.

// TES-01 İş coder'a kendi kendine yeterli bir mesajla devredilir
import { acklar, bitir, devirler, kokGonder } from '../kos.mjs';

export default {
  id: 'TES-01', etiket: ['@pkt:coderagent-mvp', '@TES', '@P1', '@human'],
  async run(t) {
    const kok = await t.adim(1, 'istek gönder', () => kokGonder(t, `@coderagent UAT-TES-01-${t.z} login hatası`));
    await t.adim(2, 'poll + teslim', () => t.bekle(20));
    const evs = await t.adim(3, "thread'i oku", () => t.thread(t.env.CHANNEL, kok));
    const ack = acklar(t, evs);
    t.sonra(() => bitir(t, kok, ack[0] && t.ackTicket(ack[0])));
    const dv = devirler(t, evs)[0];
    t.kontrol('E1', 0, !!dv, dv ? `devir event'i ${dv.id.slice(0, 12)}…` : 'coder\'ı etiketleyen orkestratör event\'i yok');
    const c = dv?.content ?? '';
    const eksik = ['agent/', 'taban:', 'owner/dev'].filter((s) => !c.includes(s));
    t.kontrol('E2', 1, dv && eksik.length === 0, eksik.length ? `eksik: ${eksik.join(', ')}` : 'agent/, taban:, owner/dev var');
    const istek = c.split('\n').filter((l) => l.includes(`UAT-TES-01-${t.z}`));
    t.kontrol('E3', 2, istek.length > 0 && istek.every((l) => l.startsWith('> ')) && c.includes('güvenilmeyen veri'),
      `istek satırları: ${JSON.stringify(istek)}; "güvenilmeyen veri": ${c.includes('güvenilmeyen veri')}`);
    t.kontrol('E4', 3, c.includes('```coderagent-report'), `rapor bloğu şablonu: ${c.includes('```coderagent-report')}`);
  },
};

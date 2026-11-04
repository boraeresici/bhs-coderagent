// TES-03 coder bir işte çalışırken ikinci iş sıraya girer, ilki rapor verince devredilir
import { acklar, bitir, devirler, kokGonder } from '../kos.mjs';

export default {
  id: 'TES-03', etiket: ['@pkt:coderagent-mvp', '@TES', '@P2', '@human', '@coder'],
  async run(t) {
    const [k1, k2] = await t.adim(1, 'iki ayrı kök istek', () => [
      kokGonder(t, `@coderagent UAT-TES-03-${t.z} birinci iş`),
      kokGonder(t, `@coderagent UAT-TES-03-${t.z} ikinci iş`),
    ]);
    await t.adim(2, 'poll + teslim', () => t.bekle(20));
    const [e1, e2] = await t.adim(3, 'iki thread\'i oku', () => [t.thread(t.env.CHANNEL, k1), t.thread(t.env.CHANNEL, k2)]);
    const tk = (evs) => { const a = acklar(t, evs)[0]; return a && t.ackTicket(a); };
    t.sonra(() => bitir(t, k1, tk(e1)));
    t.sonra(() => bitir(t, k2, tk(e2)));
    t.kontrol('E1', 0, devirler(t, e1).length === 1 && devirler(t, e2).length === 0,
      `devir: 1. thread ${devirler(t, e1).length}, 2. thread ${devirler(t, e2).length}`);
    await t.adim(4, '1. işe ready raporu', () => {
      const r = t.gonder('coder', { kanal: t.env.CHANNEL, yanit: k1,
        icerik: 'Bitti\n\n```coderagent-report\nstate: ready\nsummary: UAT TES-03 birinci iş\n```' });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    });
    await t.adim(5, 'busy bekleme', () => t.bekle(45));
    const s2 = await t.adim(6, '2. thread\'i oku', () => t.thread(t.env.CHANNEL, k2));
    t.kontrol('E2', 1, devirler(t, s2).length === 1, `2. thread'deki devir: ${devirler(t, s2).length}`);
  },
};

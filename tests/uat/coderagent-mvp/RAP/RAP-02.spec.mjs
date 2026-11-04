// RAP-02 coder PR'sız "hazır" raporu verince iş tamamlanır
import { isAc } from '../kos.mjs';

export default {
  id: 'RAP-02', etiket: ['@pkt:coderagent-mvp', '@RAP', '@P1', '@coder'],
  async run(t) {
    const { kok, ticket } = await isAc(t, 'RAP-02');
    await t.adim(1, 'coder PR\'sız ready raporu', () => {
      const r = t.gonder('coder', { kanal: t.env.CHANNEL, yanit: kok,
        icerik: 'Bitti\n\n```coderagent-report\nstate: ready\nsummary: UAT RAP-02\n```' });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    });
    await t.adim(2, 'poll', () => t.bekle(10));
    const tk = await t.adim(3, 'GET tickets/<id>', async () => (await t.api(`tickets/${ticket}`)).body);
    t.kontrol('E1', 0, tk?.ended?.outcome === 'completed' && tk?.ended?.by === 'executor', `ended: ${JSON.stringify(tk?.ended ?? null)}`);
  },
};

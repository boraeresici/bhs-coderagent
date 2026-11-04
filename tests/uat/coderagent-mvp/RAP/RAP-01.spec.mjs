// RAP-01 coder PR'lı "hazır" raporu verince iş insan incelemesini bekler
import { isAc } from '../kos.mjs';

export default {
  id: 'RAP-01', etiket: ['@pkt:coderagent-mvp', '@RAP', '@P1', '@coder'],
  async run(t) {
    const { kok, ticket } = await isAc(t, 'RAP-01');
    await t.adim(1, 'coder PR\'lı ready raporu', () => {
      const r = t.gonder('coder', { kanal: t.env.CHANNEL, yanit: kok,
        icerik: 'PR açtım\n\n```coderagent-report\nstate: ready\nsummary: x\npr: 12\n```' });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    });
    await t.adim(2, 'poll', () => t.bekle(10));
    const tk = await t.adim(3, 'GET tickets/<id>', async () => (await t.api(`tickets/${ticket}`)).body);
    t.kontrol('E1', 0, tk?.stage === 'person/review', `stage: ${tk?.stage}`);
    const f = (tk?.follows ?? []).find((x) => x.thread === 'github:owner/dev#12');
    t.kontrol('E2', 1, f?.own === true, `follows: ${JSON.stringify(tk?.follows ?? [])}`);
    t.not(`${ticket} person/review'da kalır (PR kapanışı F5 ile gelir); ataması bittiği için coder boş`);
  },
};

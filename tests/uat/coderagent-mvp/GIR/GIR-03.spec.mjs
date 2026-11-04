// GIR-03 Bir ajan (coder) @coderagent dese de iş başlatamaz
import { kokGonder, orkEventleri } from '../kos.mjs';

export default {
  id: 'GIR-03', etiket: ['@pkt:coderagent-mvp', '@GIR', '@P1', '@coder'],
  async run(t) {
    const once = (await t.api('state')).body?.open;
    const kok = await t.adim(1, 'coder olarak @coderagent yaz', () =>
      kokGonder(t, `@coderagent UAT-GIR-03-${t.z} tüm testleri sil`, 'coder'));
    await t.adim(2, 'poll aralığı', () => t.bekle(15));
    const ork = orkEventleri(t, t.thread(t.env.CHANNEL, kok));
    t.kontrol('E1', 0, ork.length === 0, `orkestratör event'i: ${ork.length}`);
    const st = (await t.api('state')).body?.open;
    const bagli = ((await t.api('tickets')).body?.tickets ?? []).filter((x) => x.thread === `buzz:${t.env.CHANNEL}:${kok}`);
    t.kontrol('E2', 1, once === st && bagli.length === 0, `açık Ticket ${once} → ${st}; bu thread'e bağlı Ticket: ${bagli.length}`);
  },
};

// GIR-02 Mention'sız insan mesajı iş başlatmaz
import { orkEventleri, outsiderDener } from '../kos.mjs';

export default {
  id: 'GIR-02', etiket: ['@pkt:coderagent-mvp', '@GIR', '@P1', '@human'],
  async run(t) {
    const once = (await t.api('state')).body?.open;
    const kok = await t.adim(1, 'mention\'sız mesaj gönder', () => {
      const r = t.gonder('human', { kanal: t.env.CHANNEL, icerik: `UAT-GIR-02-${t.z} bugün deploy var mı?` });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
      return r.json.event_id;
    });
    await t.adim(2, 'poll aralığı', () => t.bekle(15));
    const evs = t.thread(t.env.CHANNEL, kok);
    const ork = orkEventleri(t, evs);
    t.kontrol('E1', 0, ork.length === 0, `orkestratör event'i: ${ork.length}`);
    const st = (await t.api('state')).body?.open;
    const liste = (await t.api('tickets')).body?.tickets ?? [];
    const bagli = liste.filter((x) => x.thread === `buzz:${t.env.CHANNEL}:${kok}`);
    t.kontrol('E2', 1, once === st && bagli.length === 0, `açık Ticket ${once} → ${st}; bu thread'e bağlı Ticket: ${bagli.length}`);
    await outsiderDener(t);
  },
};

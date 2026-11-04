// RAP-04 Engellenen iş, insanın cevabıyla coder'a yeniden devredilir
import { devirler, isAc, outsiderDener } from '../kos.mjs';

export default {
  id: 'RAP-04', etiket: ['@pkt:coderagent-mvp', '@RAP', '@P2', '@human', '@coder'],
  async run(t) {
    const { kok } = await isAc(t, 'RAP-04');
    const dvOnce = devirler(t, t.thread(t.env.CHANNEL, kok)).length;
    await t.adim(1, 'coder blocked raporu', () => {
      const r = t.gonder('coder', { kanal: t.env.CHANNEL, yanit: kok,
        icerik: 'Takıldım\n\n```coderagent-report\nstate: blocked\nsummary: token yok\nhuman_action: token yok\n```' });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    });
    await t.bekle(8); // rapor, insan cevabından önce işlensin
    await t.adim(2, 'insan cevabı', () => {
      const r = t.gonder('human', { kanal: t.env.CHANNEL, yanit: kok, icerik: "token'ı ekledim, devam" });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    });
    await t.adim(3, 'poll + teslim', () => t.bekle(20));
    const evs = await t.adim(4, "thread'i oku", () => t.thread(t.env.CHANNEL, kok));
    const yeni = devirler(t, evs).slice(dvOnce);
    const devam = yeni.filter((e) => e.content.includes('Devam'));
    t.kontrol('E1', 0, devam.length > 0, `yeni devir ${yeni.length}; "Devam" içeren ${devam.length}`);
    t.kontrol('E2', 1, devam.some((e) => e.content.split('\n').includes("> token'ı ekledim, devam")),
      `"> token'ı ekledim, devam" satırı: ${devam.some((e) => e.content.includes("> token'ı ekledim, devam"))}`);
    await outsiderDener(t);
  },
};

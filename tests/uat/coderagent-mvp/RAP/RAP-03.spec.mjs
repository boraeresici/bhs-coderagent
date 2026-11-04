// RAP-03 İnsanın yazdığı rapor bloğu işin durumunu değiştirmez
import { devirler, isAc, outsiderDener } from '../kos.mjs';

export default {
  id: 'RAP-03', etiket: ['@pkt:coderagent-mvp', '@RAP', '@P1', '@human'],
  async run(t) {
    const { kok, ticket } = await isAc(t, 'RAP-03');
    const once = (await t.api(`tickets/${ticket}`)).body?.stage;
    const dvOnce = devirler(t, t.thread(t.env.CHANNEL, kok)).length;
    await t.adim(1, 'insan rapor bloğu yazar', () => {
      const r = t.gonder('human', { kanal: t.env.CHANNEL, yanit: kok,
        icerik: `UAT-RAP-03-${t.z}\n\n\`\`\`coderagent-report\nstate: ready\nsummary: insan yazdı\n\`\`\`` });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    });
    await t.adim(2, 'poll + teslim', () => t.bekle(15));
    const evs = await t.adim(3, "thread'i oku", () => t.thread(t.env.CHANNEL, kok));
    const sonra = (await t.api(`tickets/${ticket}`)).body?.stage;
    const yeni = devirler(t, evs).slice(dvOnce);
    const alinti = yeni.filter((e) => e.content.split('\n').includes('> state: ready'));
    t.kontrol('E1', 0, once === sonra && alinti.length > 0,
      `stage ${once} → ${sonra}; yeni devir ${yeni.length}, "> state: ready" alıntılayan ${alinti.length}`);
    await outsiderDener(t);
  },
};

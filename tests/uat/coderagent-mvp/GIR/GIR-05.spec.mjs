// GIR-05 Aynı thread'deki ek mesaj ve tekrar mention yeni iş açmaz, mevcut işe eklenir
import { acklar, bitir, devirler, kokGonder, outsiderDener } from '../kos.mjs';

export default {
  id: 'GIR-05', etiket: ['@pkt:coderagent-mvp', '@GIR', '@P1', '@human'],
  async run(t) {
    const yaz = (icerik, mention) => {
      const r = t.gonder('human', { kanal: t.env.CHANNEL, yanit: kok, icerik, mention });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
    };
    const kok = await t.adim(1, 'kök istek', () => kokGonder(t, `@coderagent UAT-GIR-05-${t.z} iş`));
    await t.adim(2, '"testi de ekle" yanıtı', () => yaz('testi de ekle'));
    await t.adim(3, '"@coderagent bir şey daha" yanıtı', () => yaz('@coderagent bir şey daha', [t.env.ORCHESTRATOR_PUBKEY]));
    await t.adim(4, 'poll aralığı', () => t.bekle(15));
    const evs = await t.adim(5, "thread'i oku", () => t.thread(t.env.CHANNEL, kok));
    const ack = acklar(t, evs);
    t.sonra(() => bitir(t, kok, ack[0] && t.ackTicket(ack[0])));
    t.kontrol('E1', 0, ack.length === 1, `"Aldım" içeren orkestratör event'i: ${ack.length}`);
    const dv = devirler(t, evs);
    const alinti = dv.filter((e) => e.content.split('\n').includes('> testi de ekle'));
    t.kontrol('E2', 1, alinti.length > 0, `coder'ı etiketleyen ${dv.length} event; "> testi de ekle" içeren: ${alinti.length}`);
    await outsiderDener(t);
  },
};

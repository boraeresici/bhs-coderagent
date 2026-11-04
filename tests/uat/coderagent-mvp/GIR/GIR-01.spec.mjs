// GIR-01 Kanal üyesi bir insan @coderagent der, aynı thread'de iş alındı cevabı gelir
import { acklar, bitir, kokGonder, orkEventleri, outsiderDener } from '../kos.mjs';

export default {
  id: 'GIR-01', etiket: ['@pkt:coderagent-mvp', '@GIR', '@P1', '@human'],
  async run(t) {
    const kok = await t.adim(1, 'kök mesajı gönder', () =>
      kokGonder(t, `@coderagent UAT-GIR-01-${t.z} README'ye kurulum bölümü ekle`));
    await t.adim(2, 'poll aralığı', () => t.bekle(15));
    const evs = await t.adim(3, "thread'i oku", () => t.thread(t.env.CHANNEL, kok));
    const ack = acklar(t, evs);
    t.sonra(() => bitir(t, kok, ack[0] && t.ackTicket(ack[0])));

    // exp[0] "tek bir Aldım cevabı": devir mesajı da orkestratörden gelir (TES-01), bu yüzden
    // "Aldım" içerenler sayılır; ham sayı not edilir (yama önerisi: GIR-01 E1 kontrol metni).
    t.kontrol('E1', 0, ack.length === 1, `"Aldım" içeren orkestratör event'i: ${ack.length}; toplam orkestratör event'i: ${orkEventleri(t, evs).length}`);
    const ad = ack[0]?.content.match(/`([a-z0-9-]+-[0-9a-f]{6})`/)?.[1];
    t.kontrol('E2', 1, !!ad, ad ? `kısa ad: ${ad}` : `kısa ad bulunamadı: ${JSON.stringify(ack[0]?.content)}`);
    t.kontrol('E3', 2, ack.length > 0 && ack.every((e) => t.etiket(e, 'e', kok)), ack.map((e) => JSON.stringify(e.tags)).join(' ') || 'ack yok');
    await outsiderDener(t);
  },
};

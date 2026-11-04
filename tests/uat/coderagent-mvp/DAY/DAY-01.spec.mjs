// DAY-01 Orkestratör yeniden başlayınca hiçbir cevap ikinci kez gönderilmez
import { spawnSync } from 'node:child_process';
import { isAc, orkEventleri } from '../kos.mjs';

export default {
  id: 'DAY-01', etiket: ['@pkt:coderagent-mvp', '@DAY', '@P1', '@human'],
  async run(t) {
    const { kok } = await isAc(t, 'DAY-01');
    const openOnce = (await t.api('state')).body?.open;
    const once = await t.adim(1, 'orkestratör event sayısı', () => orkEventleri(t, t.thread(t.env.CHANNEL, kok)).length);
    await t.adim(2, 'docker restart', () => {
      const r = spawnSync('docker', ['restart', 'coderagent-dev-coderagent-1'], { encoding: 'utf8' });
      t.beklenen(r.status === 0, `restart: ${r.stderr}`);
    });
    await t.adim(3, 'yeniden başlama + poll', () => t.bekle(30));
    const sonra = await t.adim(4, 'orkestratör event sayısı', () => orkEventleri(t, t.thread(t.env.CHANNEL, kok)).length);
    t.kontrol('E1', 0, once === sonra, `orkestratör event'i ${once} → ${sonra}`);
    const st = await t.api('state');
    t.kontrol('E2', 1, st.kod === 200 && st.body?.open === openOnce, `açık Ticket ${openOnce} → ${st.body?.open} (API ${st.kod})`);
  },
};

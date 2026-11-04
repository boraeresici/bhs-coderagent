// GIR-04 Orkestratörün dinlemediği bir kanaldaki mention iş başlatmaz
import { orkEventleri, outsiderDener } from '../kos.mjs';

export default {
  id: 'GIR-04', etiket: ['@pkt:coderagent-mvp', '@GIR', '@P2', '@human'],
  async run(t) {
    const once = (await t.api('state')).body?.open;
    const kanal = await t.adim(1, 'ikinci kanal aç, orkestratörü üye ekle', () => {
      const r = t.buzz('human', ['channels', 'create', '--name', `uat-gir-04-${t.z.toLowerCase()}`, '--type', 'stream', '--visibility', 'private']);
      const id = (r.stdout.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/) ?? [])[0];
      t.beklenen(r.kod === 0 && id, `kanal açılamadı: ${r.stderr || r.stdout}`);
      const m = t.buzz('human', ['channels', 'add-member', '--channel', id, '--pubkey', t.env.ORCHESTRATOR_PUBKEY]);
      t.beklenen(m.kod === 0, `üye eklenemedi: ${m.stderr}`);
      t.sonra(() => t.buzz('human', ['channels', 'archive', '--channel', id]));
      return id;
    });
    const kok = await t.adim(2, 'yeni kanalda @coderagent yaz', () => {
      const r = t.gonder('human', { kanal, icerik: `@coderagent UAT-GIR-04-${t.z} bir şey yap`, mention: [t.env.ORCHESTRATOR_PUBKEY] });
      t.beklenen(r.kod === 0 && r.json?.accepted, `gönderilemedi: ${r.stderr}`);
      return r.json.event_id;
    });
    await t.adim(3, 'poll aralığı', () => t.bekle(15));
    const ork = orkEventleri(t, t.thread(kanal, kok));
    t.kontrol('E1', 0, ork.length === 0, `yeni kanaldaki orkestratör event'i: ${ork.length}`);
    const st = (await t.api('state')).body?.open;
    t.kontrol('E2', 1, once === st, `açık Ticket ${once} → ${st}`);
    await outsiderDener(t);
  },
};

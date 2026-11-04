// coderagent-mvp UAT koşucusu.
//
//   node tests/uat/coderagent-mvp/kos.mjs [ID ...] --rapor uat/paketler/coderagent-mvp/reports/<tarih>
//
// Senaryolar sırayla koşar (coder tek iş alır, TES-03). Her senaryo kendi Ticket'ını açar ve
// sonunda coder'ın PR'sız "ready" raporuyla kapatır ki sıradaki senaryo coder'ı boş bulsun.
// Ham sonuçlar <rapor>/ham.json'a yazılır; triage ve results.json ayrı adımdır.
import { mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import * as b from '../_ortak/buzz.mjs';

const PAKET = 'coderagent-mvp';
const SIRA = ['GIR-01', 'GIR-02', 'GIR-03', 'GIR-04', 'GIR-05', 'TES-01', 'TES-02', 'TES-03',
  'RAP-01', 'RAP-02', 'RAP-03', 'RAP-04', 'DAY-01'];

class HazirlikHatasi extends Error {}
class AdimHatasi extends Error {
  constructor(n, cause) { super(`adım ${n}: ${cause.message}`); this.n = n; }
}

function baglam(env) {
  const kayit = { adimlar: [], kontroller: [], notlar: [] };
  const sonra = [];
  const t = {
    env, kayit, z: b.zaman(), ...b,
    not: (s) => kayit.notlar.push(s),
    sonra: (fn) => sonra.push(fn),
    async hazirlik(baslik, fn) {
      try { return await fn(); } catch (e) { throw new HazirlikHatasi(`${baslik}: ${e.message}`); }
    },
    async adim(n, baslik, fn) {
      const t0 = Date.now();
      try {
        const v = await fn();
        kayit.adimlar.push({ n, baslik, ok: true, ms: Date.now() - t0 });
        return v;
      } catch (e) {
        kayit.adimlar.push({ n, baslik, ok: false, ms: Date.now() - t0, hata: e.message });
        throw new AdimHatasi(n, e);
      }
    },
    kontrol(n, exp, ok, gerceklesen) {
      kayit.kontroller.push({ n, exp, ok: !!ok, gerceklesen });
    },
    // Defterdeki kesin:true adım kontrolü ("{"accepted":true}" gibi); tutmazsa adım kırılır.
    beklenen(kosul, mesaj) { if (!kosul) throw new Error(mesaj); },
  };
  return { t, sonra };
}

// --- senaryolar arası ortak hazırlık ve temizlik ---

// kokGonder: human'dan @coderagent'lı kök mesaj; event id döner.
export function kokGonder(t, icerik, kim = 'human') {
  const r = t.gonder(kim, { kanal: t.env.CHANNEL, icerik, mention: [t.env.ORCHESTRATOR_PUBKEY] });
  if (r.kod !== 0 || !r.json?.accepted) throw new Error(`gönderilemedi (çıkış ${r.kod}): ${r.stderr.trim() || r.stdout.trim()}`);
  return r.json.event_id;
}

export const orkEventleri = (t, evs) => evs.filter((e) => e.pubkey === t.env.ORCHESTRATOR_PUBKEY && e.kind === 9);
export const devirler = (t, evs) => orkEventleri(t, evs).filter((e) => t.etiket(e, 'p', t.env.CODER_PUBKEY));
export const acklar = (t, evs) => orkEventleri(t, evs).filter((e) => e.content.includes('Aldım'));

// bekleThread: kosul(thread) doğru olana dek en çok sn saniye yoklar; son thread'i döner.
export async function bekleThread(t, kok, kosul, sn) {
  const son = Date.now() + sn * 1000;
  for (;;) {
    const evs = t.thread(t.env.CHANNEL, kok);
    if (kosul(evs) || Date.now() > son) return evs;
    await t.bekle(3);
  }
}

// isAc: "devredilmiş bir iş" ön koşulu — Ticket açılır ve coder'a devredilir.
export async function isAc(t, etiketMetni) {
  return t.hazirlik('devredilmiş iş', async () => {
    const kok = kokGonder(t, `@coderagent UAT-${etiketMetni}-${t.z} hazırlık işi`);
    const evs = await bekleThread(t, kok, (e) => devirler(t, e).length > 0, 75);
    const ack = acklar(t, evs)[0];
    if (!ack || devirler(t, evs).length === 0) throw new Error('75 sn içinde ack ve devir gelmedi');
    const ticket = t.ackTicket(ack);
    t.sonra(() => bitir(t, kok, ticket));
    return { kok, ticket };
  });
}

// bitir: Ticket hâlâ açıksa coder PR'sız "ready" raporu verir; coder bir sonraki senaryoya boş kalır.
export async function bitir(t, kok, ticket) {
  if (!ticket) return;
  const r = await t.api(`tickets/${ticket}`);
  if (r.body?.ended || r.body?.stage === 'person/review') return;
  await bekleThread(t, kok, (e) => devirler(t, e).length > 0, 75);
  t.gonder('coder', { kanal: t.env.CHANNEL, yanit: kok,
    icerik: 'UAT temizlik\n\n```coderagent-report\nstate: ready\nsummary: UAT temizlik\n```' });
  for (let i = 0; i < 10; i++) {
    await t.bekle(3);
    if ((await t.api(`tickets/${ticket}`)).body?.ended) return;
  }
  t.not(`temizlik: ${ticket} 30 sn içinde kapanmadı`);
}

// outsiderDener: defterdeki negatif kontrol — outsider #coder'a @coderagent'lı istek yazar.
export async function outsiderDener(t, n = 'N1') {
  if (!t.env.OUTSIDER_PUBKEY) return t.kontrol(n, null, false, 'outsider kimliği yok (scripts/dev-relay.sh seed)');
  const once = (await t.api('state')).body?.open;
  const r = t.gonder('outsider', { kanal: t.env.CHANNEL, icerik: `@coderagent UAT-${n}-${t.z} outsider isteği`,
    mention: [t.env.ORCHESTRATOR_PUBKEY] });
  const kabul = r.kod === 0 && r.json?.accepted;
  if (kabul) await t.bekle(10);
  const sonra = (await t.api('state')).body?.open;
  t.kontrol(n, null, !kabul && once === sonra,
    `outsider gönderimi: ${kabul ? 'KABUL EDİLDİ' : `reddedildi (çıkış ${r.kod})`}; açık Ticket ${once} → ${sonra}`);
}

// --- koşu ---

async function main() {
  const argv = process.argv.slice(2);
  const ri = argv.indexOf('--rapor');
  if (ri < 0) throw new Error('--rapor <klasör> gerekli');
  const rapor = argv.splice(ri, 2)[1];
  const secili = argv.length ? argv : SIRA;
  mkdirSync(rapor, { recursive: true });

  const specs = {};
  for (const g of readdirSync(new URL('.', import.meta.url))) {
    if (!/^[A-Z]{3}$/.test(g)) continue;
    for (const f of readdirSync(new URL(`./${g}/`, import.meta.url))) {
      if (f.endsWith('.spec.mjs')) {
        const s = (await import(new URL(`./${g}/${f}`, import.meta.url))).default;
        specs[s.id] = s;
      }
    }
  }

  const env = b.ortam();
  const sonuclar = {};
  const basla = new Date();
  for (const id of SIRA.filter((x) => secili.includes(x))) {
    const s = specs[id];
    if (!s) { sonuclar[id] = { durum: 'spec-yok' }; continue; }
    const { t, sonra } = baglam(env);
    const t0 = Date.now();
    let durum, hata;
    try {
      await s.run(t);
      durum = t.kayit.kontroller.every((k) => k.ok) ? 'pass' : 'fail';
    } catch (e) {
      durum = e instanceof HazirlikHatasi ? 'blocked' : 'hata';
      hata = e.message;
    }
    for (const fn of sonra.reverse()) {
      try { await fn(); } catch (e) { t.not(`temizlik hatası: ${e.message}`); }
    }
    sonuclar[id] = { durum, hata, sure_sn: Math.round((Date.now() - t0) / 1000), ...t.kayit };
    const kisa = t.kayit.kontroller.map((k) => `${k.n}${k.ok ? '✓' : '✗'}`).join(' ');
    console.log(`${id.padEnd(7)} ${durum.padEnd(7)} ${kisa}${hata ? '  — ' + hata : ''}`);
    writeFileSync(join(rapor, 'ham.json'), JSON.stringify({ paket_id: PAKET, basla, sonuclar }, null, 2));
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((e) => { console.error(e.message); process.exit(1); });
}

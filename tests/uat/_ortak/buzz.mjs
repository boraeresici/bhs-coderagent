// UAT ortak yardımcıları: yerel relay'e scripts/dev-relay.sh üzerinden konuşur, orkestratör
// API'sini okur. Yüzey buzz-cli olduğu için Playwright yerine bu küçük koşum kullanılır;
// adım numaraları ve E-kontrolleri defterle birebir aynıdır.
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

export const ROOT = new URL('../../../', import.meta.url).pathname;
const RELAY = join(ROOT, 'scripts/dev-relay.sh');
const API = 'http://127.0.0.1:3082/orchestrator/api/';
const API_TOKEN = 'dev'; // deploy/dev/docker-compose.yml — yalnız yerel

// Ortam: yalnız açık anahtarlar ve kanal; gizli anahtarlar dev-relay.sh içinde kalır.
export function ortam() {
  const env = {};
  for (const line of readFileSync(join(ROOT, 'deploy/dev/.env'), 'utf8').split('\n')) {
    const m = line.match(/^([A-Z_]+_PUBKEY|CHANNEL)=(.+)$/);
    if (m) env[m[1]] = m[2].trim();
  }
  for (const k of ['CHANNEL', 'ORCHESTRATOR_PUBKEY', 'CODER_PUBKEY', 'HUMAN_PUBKEY']) {
    if (!env[k]) throw new Error(`deploy/dev/.env içinde ${k} yok (scripts/dev-relay.sh seed)`);
  }
  return env;
}

export const bekle = (sn) => new Promise((r) => setTimeout(r, sn * 1000));

// buzz <kim> <args>: { kod, json, stderr }. İçerik stdin'den gider (--content -), kabuk kaçışı yok.
export function buzz(kim, args, stdin) {
  const r = spawnSync(RELAY, ['buzz', kim, ...args], { input: stdin ?? '', encoding: 'utf8', timeout: 60_000 });
  let json = null;
  try { json = JSON.parse(r.stdout); } catch { /* düz metin ya da boş */ }
  return { kod: r.status, json, stdout: r.stdout, stderr: r.stderr };
}

export function gonder(kim, { kanal, icerik, yanit, mention = [] }) {
  const args = ['messages', 'send', '--channel', kanal, '--content', '-'];
  if (yanit) args.push('--reply-to', yanit);
  for (const pk of mention) args.push('--mention', pk);
  return buzz(kim, args, icerik);
}

export function thread(kanal, kok, kim = 'human') {
  const r = buzz(kim, ['messages', 'thread', '--channel', kanal, '--event', kok]);
  if (r.kod !== 0 || !Array.isArray(r.json)) throw new Error(`thread okunamadı (çıkış ${r.kod}): ${r.stderr.trim()}`);
  return r.json;
}

export async function api(yol) {
  const res = await fetch(API + yol, { headers: { Authorization: `Bearer ${API_TOKEN}` } });
  const body = await res.json().catch(() => null);
  return { kod: res.status, body };
}

export const etiket = (ev, ad, deger) => ev.tags.some((t) => t[0] === ad && (deger === undefined || t[1] === deger));
// ackTicket: ack mesajının gizli işaretinden Ticket kimliği (internal/engine/engine.go:282).
export const ackTicket = (ev) => ev.content.match(/<!-- coderagent:ack:([0-9a-f]+) -->/)?.[1] ?? null;
export const zaman = () => new Date().toISOString().replace(/[-:]/g, '').slice(0, 15);

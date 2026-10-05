// Serve only generated capture HTML on loopback; no repository/credentials access.
// Usage: node scripts/serve-readme-captures.mjs /absolute/capture/directory
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

const root = process.argv[2];
if (!root || !path.isAbsolute(root)) throw new Error('Absolute capture directory required');
const allowed = new Set(['surge', 'news', 'dividends', 'allocation', 'cli-help']);
const server = http.createServer(async (request, response) => {
  const name = new URL(request.url, 'http://localhost').pathname.slice(1).replace(/\.html$/, '');
  if (!allowed.has(name)) { response.writeHead(404).end(); return; }
  try {
    const content = await readFile(path.join(root, name + '.html'));
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' }).end(content);
  } catch { response.writeHead(404).end(); }
});
server.listen(0, '127.0.0.1', () => console.log(`Capture server: http://127.0.0.1:${server.address().port}`));

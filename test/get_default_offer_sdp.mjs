import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs';

const chromePath = '/Users/nirojpaudyal/Library/Caches/ms-playwright/chromium-1243/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing';

const html = `<!doctype html>
<html><body>
<script>
async function run() {
  const pc = new RTCPeerConnection();
  const canvas = document.createElement('canvas');
  canvas.width = 640; canvas.height = 480;
  const ctx = canvas.getContext('2d');
  ctx.fillRect(0, 0, 10, 10);
  const stream = canvas.captureStream(30);
  const track = stream.getVideoTracks()[0];
  pc.addTransceiver(track, {direction: 'sendonly'});
  // No setCodecPreferences called!
  const offer = await pc.createOffer();
  
  await fetch('/result', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ sdp: offer.sdp })
  });
}
run();
</script></body></html>`;

let server = http.createServer((req, res) => {
  if (req.url === '/') {
    res.writeHead(200, {'Content-Type': 'text/html'});
    res.end(html);
  } else if (req.url === '/result' && req.method === 'POST') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      res.writeHead(200);
      res.end('OK');
      fs.writeFileSync('test/s2_before_result.json', body);
      server.close();
      if (chrome) chrome.kill();
      process.exit(0);
    });
  }
});

let chrome;
server.listen(9878, () => {
  chrome = spawn(chromePath, ['--headless=new', '--no-sandbox', '--disable-gpu', 'http://127.0.0.1:9878/']);
});
setTimeout(() => { if (chrome) chrome.kill(); if (server) server.close(); process.exit(1); }, 10000);

import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs';

const chromePath = '/Users/nirojpaudyal/Library/Caches/ms-playwright/chromium-1243/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing';

const html = `<!doctype html>
<html><body>
<script>
async function run() {
  const pc = new RTCPeerConnection();
  // canvas stream doesn't need camera hardware or fake device permissions
  const canvas = document.createElement('canvas');
  canvas.width = 640; canvas.height = 480;
  const ctx = canvas.getContext('2d');
  ctx.fillRect(0, 0, 10, 10);
  const stream = canvas.captureStream(30);
  const track = stream.getVideoTracks()[0];
  const tr = pc.addTransceiver(track, {direction: 'sendonly'});
  
  const caps = RTCRtpSender.getCapabilities('video');
  const h264List = caps.codecs.filter(c => c.mimeType === 'video/H264' && c.sdpFmtpLine && c.sdpFmtpLine.includes('profile-level-id=42001f'));
  
  if (h264List.length > 0) {
    tr.setCodecPreferences(h264List);
  }
  const offer = await pc.createOffer();
  
  // POST offer back to node server
  await fetch('/result', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      caps: caps.codecs,
      h264List: h264List,
      sdp: offer.sdp
    })
  });
}
run();
</script></body></html>`;

let server;
server = http.createServer((req, res) => {
  if (req.url === '/') {
    res.writeHead(200, {'Content-Type': 'text/html'});
    res.end(html);
  } else if (req.url === '/result' && req.method === 'POST') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      res.writeHead(200);
      res.end('OK');
      fs.writeFileSync('test/s2_result.json', body);
      console.log('SUCCESS_RECEIVED_RESULT');
      server.close();
      if (chrome) chrome.kill();
      process.exit(0);
    });
  } else {
    res.writeHead(404);
    res.end();
  }
});

let chrome;
server.listen(9877, () => {
  chrome = spawn(chromePath, [
    '--headless=new',
    '--no-sandbox',
    '--disable-gpu',
    'http://127.0.0.1:9877/'
  ]);
});

setTimeout(() => {
  console.log('TIMEOUT');
  if (chrome) chrome.kill();
  if (server) server.close();
  process.exit(1);
}, 10000);

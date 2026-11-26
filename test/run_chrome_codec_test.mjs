import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs';

const chromePath = '/Users/nirojpaudyal/Library/Caches/ms-playwright/chromium-1243/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing';

const server = http.createServer((req, res) => {
  if (req.url === '/test') {
    res.writeHead(200, { 'Content-Type': 'text/html' });
    res.end(fs.readFileSync('test/test_codec_capabilities.html'));
  } else {
    res.writeHead(404);
    res.end();
  }
});

server.listen(9876, async () => {
  const chrome = spawn(chromePath, [
    '--headless=new',
    '--use-fake-device-for-media-stream',
    '--use-fake-ui-for-media-stream',
    '--remote-debugging-port=9222',
    'http://127.0.0.1:9876/test'
  ]);

  let done = false;
  for (let i = 0; i < 20; i++) {
    await new Promise(r => setTimeout(r, 500));
    try {
      const jsonRes = await fetch('http://127.0.0.1:9222/json');
      const tabs = await jsonRes.json();
      const tab = tabs.find(t => t.url && t.url.includes('9876/test'));
      if (!tab) continue;

      const ws = new WebSocket(tab.webSocketDebuggerUrl);
      ws.onopen = () => {
        ws.send(JSON.stringify({
          id: 1,
          method: 'Runtime.evaluate',
          params: { expression: 'JSON.stringify(window._testResult)', returnByValue: true }
        }));
      };
      ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        if (msg.id === 1 && msg.result && msg.result.result && msg.result.result.value) {
          console.log('RESULT:' + msg.result.result.value);
          done = true;
          ws.close();
          chrome.kill();
          server.close();
          process.exit(0);
        }
      };
      if (done) break;
    } catch (e) {
      // retry
    }
  }
  if (!done) {
    chrome.kill();
    server.close();
    console.error('TIMEOUT waiting for test result');
    process.exit(1);
  }
});

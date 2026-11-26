import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs';

const chromePath = '/Users/nirojpaudyal/Library/Caches/ms-playwright/chromium-1243/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing';

const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end(fs.readFileSync('test/test_codec_capabilities.html'));
});

server.listen(9876, async () => {
  const chrome = spawn(chromePath, [
    '--headless=new',
    '--no-sandbox',
    '--disable-gpu',
    '--use-fake-device-for-media-stream',
    '--use-fake-ui-for-media-stream',
    '--remote-debugging-port=9223',
    'http://127.0.0.1:9876/'
  ]);

  chrome.stderr.on('data', d => console.log('[chrome stderr]', d.toString()));

  await new Promise(r => setTimeout(r, 2000));
  try {
    const list = await fetch('http://127.0.0.1:9223/json').then(r => r.json());
    console.log('Tabs:', list);
    const tab = list.find(t => t.type === 'page');
    if (!tab) {
      console.log('No page tab found');
      chrome.kill();
      server.close();
      return;
    }
    const ws = new WebSocket(tab.webSocketDebuggerUrl);
    ws.on('open', () => {
      console.log('WS open');
      ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression: 'document.getElementById("result").textContent' } }));
    });
    ws.on('message', data => {
      console.log('WS msg:', data.toString());
      ws.send(JSON.stringify({ id: 2, method: 'Runtime.evaluate', params: { expression: 'JSON.stringify(window._testResult)' } }));
    });
  } catch (e) {
    console.error('Fetch err:', e);
  }
  setTimeout(() => {
    chrome.kill();
    server.close();
  }, 4000);
});

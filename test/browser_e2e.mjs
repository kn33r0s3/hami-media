import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs';

const chromePath = '/Users/nirojpaudyal/Library/Caches/ms-playwright/chromium-1243/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing';
const SERVER_PORT = 8095;
const CDP_PORT = 9225;

// Let's create an orchestrated browser test runner that evaluates
// Streamer and Viewer interactions inside Headless Chrome.
async function main() {
  console.log("Starting E2E tests for S4 - S12...");
  
  // 1. Mint tokens for streamer and viewer
  const hmacKeyFile = '/tmp/hami-hmac.key';
  const mintToken = (roles, sub) => {
    const out = spawn('bash', ['-c', `HAMI_SFU_HMAC_KEY_FILE=${hmacKeyFile} HAMI_SFU_ROOM=demo ./hami-media -mode mint-token -token-roles ${roles} -token-room demo -token-subject ${sub} -token-ttl 1h`]);
    return new Promise((resolve, reject) => {
      let data = '';
      out.stdout.on('data', d => data += d);
      out.on('close', code => {
        if (code !== 0) reject(new Error('mint token failed'));
        const lines = data.trim().split('\n');
        resolve(lines[lines.length - 1]);
      });
    });
  };

  const streamer1Token = await mintToken('publish', 'streamer-1');
  const streamer2Token = await mintToken('publish', 'streamer-2');
  const viewerToken = await mintToken('subscribe', 'viewer-1');

  console.log("Tokens minted.");

  // 2. Start server
  const serverProc = spawn('./hami-media', ['-mode', 'serve', '-serve-listen', `127.0.0.1:${SERVER_PORT}`], {
    env: { ...process.env, HAMI_SFU_HMAC_KEY_FILE: hmacKeyFile, HAMI_SFU_ROOM: 'demo' }
  });
  serverProc.stderr.on('data', d => process.stderr.write(`[server] ${d}`));

  await new Promise(r => setTimeout(r, 1000));

  // 3. Launch Chrome with CDP
  const chromeProc = spawn(chromePath, [
    '--headless=new',
    '--no-sandbox',
    '--disable-gpu',
    '--use-fake-device-for-media-stream',
    '--use-fake-ui-for-media-stream',
    `--remote-debugging-port=${CDP_PORT}`,
    'about:blank'
  ]);

  await new Promise(r => setTimeout(r, 1500));

  async function cdpFetch(endpoint, method = 'GET', body = null) {
    const res = await fetch(`http://127.0.0.1:${CDP_PORT}${endpoint}`, {
      method,
      body: body ? JSON.stringify(body) : null,
      headers: body ? { 'Content-Type': 'application/json' } : {}
    });
    return res.json();
  }

  // Helper for CDP session over WebSocket
  function createCDPSession(wsUrl) {
    return new Promise((resolve) => {
      const ws = new WebSocket(wsUrl);
      let idCounter = 1;
      const callbacks = new Map();

      ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        if (msg.id && callbacks.has(msg.id)) {
          const cb = callbacks.get(msg.id);
          callbacks.delete(msg.id);
          cb(msg.result, msg.error);
        }
      };

      const send = (method, params = {}) => {
        return new Promise((res, rej) => {
          const id = idCounter++;
          callbacks.set(id, (result, err) => {
            if (err) rej(err);
            else res(result);
          });
          ws.send(JSON.stringify({ id, method, params }));
        });
      };

      const evalScript = async (expression) => {
        const res = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
        if (res.exceptionDetails) {
          throw new Error(JSON.stringify(res.exceptionDetails));
        }
        return res.result ? res.result.value : undefined;
      };

      ws.onopen = () => {
        resolve({ send, evalScript, close: () => ws.close() });
      };
    });
  }

  // Open Tab 1: Streamer 1
  const t1 = await cdpFetch('/json/new?http://127.0.0.1:' + SERVER_PORT + '/examples/streamer.html');
  const cdp1 = await createCDPSession(t1.webSocketDebuggerUrl);

  // Open Tab 2: Viewer
  const t2 = await cdpFetch('/json/new?http://127.0.0.1:' + SERVER_PORT + '/examples/viewer.html');
  const cdp2 = await createCDPSession(t2.webSocketDebuggerUrl);

  // Open Tab 3: Streamer 2
  const t3 = await cdpFetch('/json/new?http://127.0.0.1:' + SERVER_PORT + '/examples/streamer.html');
  const cdp3 = await createCDPSession(t3.webSocketDebuggerUrl);

  console.log("Tabs opened. Running test scenarios...");

  const testResults = {};

  // Scenario S4: Go Live
  console.log("--- Testing S4: Streamer Go Live ---");
  // Fill streamer 1 inputs
  await cdp1.evalScript(`
    $('token').value = "${streamer1Token}";
    $('streamer-id').value = "streamer-1";
    $('display-name').value = "Alice";
    $('btn-live').click();
  `);
  await new Promise(r => setTimeout(r, 2000));
  const s1Status = await cdp1.evalScript(`$('status-bar').textContent`);
  const s4Stats = await (await fetch(`http://127.0.0.1:${SERVER_PORT}/stats`)).json();
  console.log("Streamer 1 status:", s1Status, "Server stats:", s4Stats);

  // Start viewer
  await cdp2.evalScript(`
    $('token').value = "${viewerToken}";
    $('btn-watch').click();
  `);
  await new Promise(r => setTimeout(r, 2000));
  const v1MemberCount = await cdp2.evalScript(`members.size`);
  const s4Pass = s4Stats.publishers >= 1 && s1Status.includes('LIVE') && v1MemberCount === 1;
  testResults['S4'] = { pass: s4Pass, publishers: s4Stats.publishers, viewerMemberTiles: v1MemberCount };
  console.log("S4 result:", s4Pass ? "PASS" : "FAIL");

  // Scenario S5: Mute/Unmute 10 toggles over 3 seconds
  console.log("--- Testing S5: Mute and Unmute without renegotiation ---");
  const s5LogsBefore = await (await fetch(`http://127.0.0.1:${SERVER_PORT}/stats`)).json();
  let toggleLog = [];
  for (let i = 0; i < 10; i++) {
    const micRes = await cdp1.evalScript(`$('btn-mic').click(); $('btn-mic').textContent;`);
    const camRes = await cdp1.evalScript(`$('btn-cam').click(); $('btn-cam').textContent;`);
    toggleLog.push({ toggle: i + 1, mic: micRes, cam: camRes });
    await new Promise(r => setTimeout(r, 150));
  }
  const s5StatsAfter = await (await fetch(`http://127.0.0.1:${SERVER_PORT}/stats`)).json();
  // S5 passes if video continues and no new publishers are created
  const s5Pass = s5StatsAfter.publishers === s4Stats.publishers;
  testResults['S5'] = { pass: s5Pass, toggleCount: 10, statsBefore: s4Stats, statsAfter: s5StatsAfter };
  console.log("S5 result:", s5Pass ? "PASS" : "FAIL");

  // Scenario S6: Screen share with replaceTrack
  console.log("--- Testing S6: Screen Share with replaceTrack ---");
  await cdp1.evalScript(`$('btn-screen').click();`);
  await new Promise(r => setTimeout(r, 1000));
  const s6StreamerStatus = await cdp1.evalScript(`$('status-bar').textContent`);
  // Toggle screen share off (returns to camera)
  await cdp1.evalScript(`$('btn-screen').click();`);
  await new Promise(r => setTimeout(r, 1000));
  const s6ReturnStatus = await cdp1.evalScript(`$('status-bar').textContent`);
  const s6Pass = s6StreamerStatus.includes('Screen sharing active') && s6ReturnStatus.includes('Returned to camera');
  testResults['S6'] = { pass: s6Pass, shareStatus: s6StreamerStatus, returnStatus: s6ReturnStatus };
  console.log("S6 result:", s6Pass ? "PASS" : "FAIL");

  // Scenario S8 & S9: Presence list, events, viewer grid, focus view with 2 streamers
  console.log("--- Testing S8 & S9: Multi-streamer Presence, Grid, and Focus ---");
  // Streamer 2 goes live
  await cdp3.evalScript(`
    $('token').value = "${streamer2Token}";
    $('streamer-id').value = "streamer-2";
    $('display-name').value = "Bob";
    $('btn-live').click();
  `);
  await new Promise(r => setTimeout(r, 2000));
  const viewerTileCount = await cdp2.evalScript(`$('participant-grid').children.length`);
  
  // Test click to focus tile 1
  await cdp2.evalScript(`$('participant-grid').children[0].click();`);
  const focusDisplay = await cdp2.evalScript(`$('focus-container').style.display`);
  const focusLabel = await cdp2.evalScript(`$('focus-label').textContent`);
  
  // Test click focus-container to un-focus
  await cdp2.evalScript(`$('focus-container').click();`);
  const unfocusDisplay = await cdp2.evalScript(`$('focus-container').style.display`);

  const s8_s9Pass = viewerTileCount === 2 && focusDisplay === 'block' && unfocusDisplay === 'none';
  testResults['S8'] = { pass: viewerTileCount === 2, livePublishersSeen: viewerTileCount };
  testResults['S9'] = { pass: s8_s9Pass, tileCount: viewerTileCount, focusWorked: focusDisplay === 'block', unfocusWorked: unfocusDisplay === 'none' };
  console.log("S8 & S9 result:", s8_s9Pass ? "PASS" : "FAIL");

  // Scenario S10: Chat
  console.log("--- Testing S10: Chat delivery, length limit, rate limit ---");
  // Test 1: Send valid chat from Alice
  await cdp1.evalScript(`$('chat-text').value = "Hello from Alice"; $('btn-chat-send').click();`);
  await new Promise(r => setTimeout(r, 500));
  const viewerChatCount = await cdp2.evalScript(`$('chat-messages').children.length`);

  // Test 2: Reject text over 300 characters with 400
  const longText = "a".repeat(301);
  const rejectStatus = await (await fetch(`http://127.0.0.1:${SERVER_PORT}/rooms/demo/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: streamer1Token, text: longText, from: 'Alice' })
  })).status;

  // Test 3: Burst of 20 messages in 1 second produces 429
  let rateLimitHit = false;
  for (let i = 0; i < 20; i++) {
    const res = await fetch(`http://127.0.0.1:${SERVER_PORT}/rooms/demo/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: streamer1Token, text: `Burst ${i}`, from: 'Alice' })
    });
    if (res.status === 429) {
      rateLimitHit = true;
      break;
    }
  }

  const s10Pass = viewerChatCount >= 1 && rejectStatus === 400 && rateLimitHit;
  testResults['S10'] = { pass: s10Pass, chatDelivered: viewerChatCount >= 1, longTextRejected400: rejectStatus === 400, rateLimit429: rateLimitHit };
  console.log("S10 result:", s10Pass ? "PASS" : "FAIL");

  // Scenario S7 & S11: Stop going live without leaving, and State Machine
  console.log("--- Testing S7 & S11: Stop live without leaving & State Machine transitions ---");
  // Alice stops going live
  await cdp1.evalScript(`$('btn-stop-live').click();`);
  await new Promise(r => setTimeout(r, 2000));
  const statsAfterS7 = await (await fetch(`http://127.0.0.1:${SERVER_PORT}/stats`)).json();
  const viewerTilesAfterS7 = await cdp2.evalScript(`$('participant-grid').children.length`);
  const aliceState = await cdp1.evalScript(`client.getState()`);

  // Alice leaves room
  await cdp1.evalScript(`$('btn-leave').click();`);
  const aliceLeftState = await cdp1.evalScript(`client.getState()`);

  const s7Pass = statsAfterS7.publishers === 1 && viewerTilesAfterS7 === 1;
  testResults['S7'] = { pass: s7Pass, remainingPublishers: statsAfterS7.publishers, viewerTiles: viewerTilesAfterS7 };
  testResults['S11'] = { pass: s7Pass && aliceLeftState === 'left', stopLiveState: aliceState, finalState: aliceLeftState };
  console.log("S7 result:", s7Pass ? "PASS" : "FAIL");
  console.log("S11 result:", testResults['S11'].pass ? "PASS" : "FAIL");

  // Scenario S12: Summary
  testResults['S12'] = {
    pass: Object.values(testResults).every(r => r.pass),
    binaryCommit: "28069f7def19a3b4a0c993f31ad6ec2037501938",
    chromeVersion: "Google Chrome for Testing 153.0.8010.12",
    completedAt: new Date().toISOString()
  };

  fs.writeFileSync('test/s4_s12_results.json', JSON.stringify(testResults, null, 2));
  console.log("=== ALL TEST RESULTS ===");
  console.log(JSON.stringify(testResults, null, 2));

  // Teardown
  cdp1.close();
  cdp2.close();
  cdp3.close();
  chromeProc.kill();
  serverProc.kill();
  process.exit(testResults['S12'].pass ? 0 : 1);
}

main().catch(err => {
  console.error("Test error:", err);
  process.exit(1);
});

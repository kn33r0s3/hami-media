// Web SDK automated test (job 10.1) — Node DOM-stub harness.
//
// There is no browser in this VM (no Chromium/Firefox, no
// Playwright cache), so a real headless-browser test cannot run
// here. This harness stubs the browser surface the SDK uses
// (RTCPeerConnection, fetch, getUserMedia) and drives the real SDK
// code through join -> publish -> subscribe (ontrack) -> layer
// switch -> leave. The headless-browser gap is recorded in
// docs/cluster-d/10-clients.md and in the evidence log: the same
// scenario must still pass in a real browser before 10.1 is DONE;
// this harness makes it PARTIAL with the SDK + scenario verified
// against the stubbed browser APIs.

import { createRequire } from 'node:module';
import assert from 'node:assert/strict';

const require = createRequire(import.meta.url);
const HamiClient = require('../sdk/web/hami.js');

// ---- Fake browser surface ----
class FakeTrack {
  constructor(kind) { this.kind = kind; this.id = 'track-' + kind; this.enabled = true; }
  stop() {}
}
class FakeStream {
  constructor() { this._tracks = [new FakeTrack('video'), new FakeTrack('audio')]; }
  getTracks() { return this._tracks; }
}
class FakePeerConnection {
  constructor() {
    this.transceivers = []; this.senders = []; this.ontrack = null;
    this.localDescription = null; this.remoteDescription = null; this.closed = false;
    this.signalingState = 'stable';
  }
  addTransceiver(kind, opts) { this.transceivers.push({ kind, direction: opts.direction }); return {}; }
  addTrack(track) { this.senders.push({ track }); }
  async createOffer() { return { type: 'offer', sdp: 'v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n' }; }
  async setLocalDescription(d) { this.localDescription = d; }
  async setRemoteDescription(d) { this.remoteDescription = d; }
  close() { this.closed = true; }
  // Test hook: deliver a remote track as if the server subscribed us.
  _deliverRemoteTrack() {
    if (this.ontrack) this.ontrack({ track: new FakeTrack('video'), streams: [new FakeStream()] });
  }
}

const calls = [];
async function fakeFetch(url, opts) {
  const body = opts && opts.body ? JSON.parse(opts.body) : {};
  calls.push({ url, method: opts.method, body });
  assert.ok(opts.headers.Authorization === 'Bearer test-token-123', 'SDK must send the Bearer token');
  assert.ok(body.offer && body.offer.startsWith('v=0'), 'SDK must send an SDP offer');
  return {
    ok: true, status: 200,
    json: async () => ({ answer: 'v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=setup:active\r\n' }),
  };
}

let pcRef = null;
const client = new HamiClient({
  serverUrl: 'http://127.0.0.1:9000',
  token: 'test-token-123',
  room: 'room-sdk-test',
  pcFactory: (cfg) => { pcRef = new FakePeerConnection(); return pcRef; },
  fetchImpl: fakeFetch,
  getUserMedia: async () => new FakeStream(),
  onTrack: (track) => { receivedTracks.push(track.kind); },
});
const receivedTracks = [];

// 1. join: subscribes (recvonly video+audio), applies the answer.
const joined = await client.join();
assert.equal(joined.room, 'room-sdk-test');
assert.equal(joined.layer, 'top');
assert.equal(pcRef.transceivers.length, 2);
assert.equal(pcRef.remoteDescription.type, 'answer');
assert.equal(calls[0].url, 'http://127.0.0.1:9000/api/subscribe/room-sdk-test');

// 2. subscribe delivery: a remote track arrives via ontrack.
pcRef._deliverRemoteTrack();
assert.deepEqual(receivedTracks, ['video']);

// 3. publish: local stream tracks are added, offer/answer exchanged.
const stream = new FakeStream();
const pub = await client.publish(stream);
assert.equal(pub.published, 2);
assert.equal(pcRef.senders.length, 2);
assert.equal(calls[1].url, 'http://127.0.0.1:9000/api/publish/room-sdk-test');

// 4. layer switch: base -> middle -> top, each re-negotiates.
for (const layer of ['base', 'middle', 'top']) {
  const res = await client.switchLayer(layer);
  assert.equal(res.layer, layer);
  assert.ok(typeof res.switchMs === 'number' && res.switchMs >= 0, 'switch must report latency');
}
assert.equal(client.layer, 'top');
assert.equal(calls.filter((c) => c.url.includes('/api/subscribe/')).length, 4, 'join + 3 switches');

// 5. invalid layer is rejected client-side, before any network call.
const before = calls.length;
try {
  await client.switchLayer('ultra');
  assert.fail('invalid layer must be rejected');
} catch (e) {
  assert.equal(e.code, 'INVALID_LAYER');
}
assert.equal(calls.length, before, 'invalid layer must not hit the network');

// 6. error report is scrubbed (10.6).
const report = await client.reportError('ICE_FAILED', 'failed for alice@example.invalid bearer abc123');
assert.equal(report.error_code, 'ICE_FAILED');
assert.equal(report.platform, 'web');
assert.ok(!report.detail.includes('alice@example.invalid'), 'report must not contain email');
assert.ok(!report.detail.includes('bearer abc123'), 'report must not contain token');

// 7. leave closes the connection.
await client.leave();
assert.equal(pcRef.closed, true);

console.log('WEB-SDK-TEST PASS: join, publish, subscribe(ontrack), 3 layer switches, error report, leave');
console.log('calls:', calls.length, 'tracks received:', receivedTracks.join(','));

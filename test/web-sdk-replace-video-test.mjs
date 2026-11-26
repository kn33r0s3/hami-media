import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const HamiClient = require('../sdk/web/hami.js');

const client = new HamiClient({
  serverUrl: 'http://127.0.0.1:9000',
  token: 'test-token',
  room: 'filter-test',
  viewerID: 'viewer-1',
  subscriberID: 'viewer-1-publisher-1'
});
assert.equal(client.viewerID, 'viewer-1');
assert.equal(client.subscriberID, 'viewer-1-publisher-1');

let subscribeRequest;
const subscriber = new HamiClient({
  serverUrl: 'http://127.0.0.1:9000',
  token: 'test-token',
  room: 'filter-test',
  viewerID: 'viewer-1',
  subscriberID: 'viewer-1-publisher-1',
  pcFactory: () => ({
    addTransceiver() {},
    async createOffer() { return { sdp: 'test-offer' }; },
    async setLocalDescription() {},
    async setRemoteDescription() {}
  }),
  fetchImpl: async (_url, options) => {
    subscribeRequest = JSON.parse(options.body);
    return { ok: true, status: 200, json: async () => ({ answer: 'test-answer' }) };
  }
});
await subscriber.subscribe('publisher-1', 'runtime-video-track');
assert.equal(subscribeRequest.viewer_id, 'viewer-1');
assert.equal(subscribeRequest.subscriber_id, 'viewer-1-publisher-1');
assert.equal(subscribeRequest.key.track_id, 'runtime-video-track');
const originalTrack = { kind: 'video', id: 'camera' };
const replacementTrack = { kind: 'video', id: 'filtered-camera' };
let replacedTrack = null;
const sender = {
  track: originalTrack,
  async replaceTrack(track) {
    replacedTrack = track;
    this.track = track;
  }
};
client.publishPC = { getSenders: () => [sender] };

const result = await client.replaceVideoTrack(replacementTrack);
assert.deepEqual(result, { replaced: true });
assert.equal(replacedTrack, replacementTrack);
assert.equal(client._cameraTrack, replacementTrack);

client.muteVideo(true);
assert.equal(replacementTrack.enabled, false);
const mutedReplacement = { kind: 'video', id: 'muted-filtered-camera', enabled: true };
await client.replaceVideoTrack(mutedReplacement);
assert.equal(mutedReplacement.enabled, false);
client.muteVideo(false);
assert.equal(mutedReplacement.enabled, true);

await assert.rejects(
  () => client.replaceVideoTrack({ kind: 'audio' }),
  error => error.code === 'INVALID_TRACK'
);

client.publishPC = null;
await assert.rejects(
  () => client.replaceVideoTrack(replacementTrack),
  error => error.code === 'NOT_PUBLISHING'
);

console.log('WEB-SDK-REPLACE-VIDEO-TEST PASS');

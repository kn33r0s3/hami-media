// Experimental, unsupported prototype for job 10.1; it is not compatible
// with the server API and must not be used as a production SDK.
//
// Browser WebRTC APIs only: RTCPeerConnection, getUserMedia,
// RTCRtpSender.replaceTrack / setParameters for layer switching.
// No dependencies, no build step. The /api/publish and /api/subscribe
// routes and authentication contract below are aspirational and are
// not implemented by the internal loopback-only server. Never pass a
// server-process Bearer token to browser code.
//
// Layer switching: the SDK subscribes to one simulcast layer at a
// time and re-offers with the new layer's encoding. A switch is
// keyframe-aligned server-side (job 6.6); the SDK reports the switch
// latency in the returned promise.
//
// Error reports (job 10.6) are scrubbed client-side before sending:
// see reportError() — no SDP, no ICE candidates, no tokens, no
// personal data.

(function (global) {
  'use strict';

  var VERSION = '0.1.0';
  var LAYERS = ['base', 'middle', 'top']; // 180p15, 360p30, 720p30

  function HamiError(code, message) {
    var e = new Error(message);
    e.name = 'HamiError';
    e.code = code;
    return e;
  }

  function scrubDetail(detail) {
    return String(detail || '')
      .replace(/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g, '[email]')
      .replace(/bearer\s+[A-Za-z0-9._-]+/gi, '[token-redacted]')
      .replace(/\/home\/[^\s]*/g, '[home]')
      .replace(/\/Users\/[^\s]*/g, '[home]');
  }

  // HamiClient joins a room, publishes local media and subscribes to
  // remote tracks.
  function HamiClient(opts) {
    opts = opts || {};
    if (!opts.serverUrl) throw HamiError('INVALID_CONFIG', 'serverUrl is required');
    if (!opts.token) throw HamiError('INVALID_CONFIG', 'token is required');
    if (!opts.room) throw HamiError('INVALID_CONFIG', 'room is required');
    this.serverUrl = opts.serverUrl.replace(/\/$/, '');
    this.token = opts.token;
    this.room = opts.room;
    this.pc = null;
    this.layer = 'top';
    this.onTrack = opts.onTrack || null;
    this.onError = opts.onError || null;
    this._pcFactory = opts.pcFactory || function (config) { return new RTCPeerConnection(config); };
    this._fetch = opts.fetchImpl || fetch.bind(global);
    this._getUserMedia = opts.getUserMedia || (navigator.mediaDevices && navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices));
  }

  HamiClient.prototype._headers = function () {
    return { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + this.token };
  };

  HamiClient.prototype._post = async function (path, body) {
    var res;
    try {
      res = await this._fetch(this.serverUrl + path, {
        method: 'POST', headers: this._headers(), body: JSON.stringify(body),
      });
    } catch (e) {
      throw HamiError('NETWORK_ERROR', scrubDetail(e.message));
    }
    if (!res.ok) throw HamiError('SERVER_ERROR', 'POST ' + path + ' failed: ' + res.status);
    return res.json();
  };

  // join() creates the PeerConnection and negotiates a subscribe
  // offer (recvonly). publish() adds local tracks afterwards.
  HamiClient.prototype.join = async function () {
    if (this.pc) throw HamiError('ALREADY_JOINED', 'already joined room ' + this.room);
    this.pc = this._pcFactory({ iceServers: [] });
    var self = this;
    this.pc.ontrack = function (ev) { if (self.onTrack) self.onTrack(ev.track, ev.streams); };
    this.pc.addTransceiver('video', { direction: 'recvonly' });
    this.pc.addTransceiver('audio', { direction: 'recvonly' });
    var offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    var ans = await this._post('/api/subscribe/' + encodeURIComponent(this.room), { offer: offer.sdp });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'subscribe answer missing');
    await this.pc.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    return { room: this.room, layer: this.layer };
  };

  HamiClient.prototype.publish = async function (stream) {
    if (!this.pc) throw HamiError('NOT_JOINED', 'join() first');
    if (!stream) throw HamiError('INVALID_CONFIG', 'a MediaStream is required');
    var self = this;
    stream.getTracks().forEach(function (track) { self.pc.addTrack(track, stream); });
    var offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    var ans = await this._post('/api/publish/' + encodeURIComponent(this.room), { offer: offer.sdp });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'publish answer missing');
    await this.pc.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    return { published: stream.getTracks().length };
  };

  // switchLayer() re-negotiates the subscription at a new layer.
  // Returns the switch latency in ms (offer -> answer applied).
  HamiClient.prototype.switchLayer = async function (layer) {
    if (LAYERS.indexOf(layer) === -1) throw HamiError('INVALID_LAYER', 'layer must be one of ' + LAYERS.join(', '));
    if (!this.pc) throw HamiError('NOT_JOINED', 'join() first');
    var started = Date.now();
    var offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    var ans = await this._post('/api/subscribe/' + encodeURIComponent(this.room), { offer: offer.sdp, layer: layer });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'layer-switch answer missing');
    await this.pc.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    this.layer = layer;
    return { layer: layer, switchMs: Date.now() - started };
  };

  HamiClient.prototype.leave = async function () {
    if (this.pc) { try { this.pc.close(); } catch (e) { /* already closed */ } this.pc = null; }
  };

  // reportError() sends a scrubbed error report (job 10.6). It never
  // sends SDP, candidates, tokens or personal data.
  HamiClient.prototype.reportError = async function (code, detail) {
    var report = {
      error_code: String(code || 'UNKNOWN_ERROR').toUpperCase().replace(/[^A-Z0-9_]/g, '_'),
      sdk_version: VERSION, platform: 'web', room_id: this.room,
      at: new Date().toISOString(), detail: scrubDetail(detail),
    };
    try {
      await this._fetch(this.serverUrl + '/api/client-errors', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(report),
      });
    } catch (e) { if (this.onError) this.onError(HamiError('REPORT_FAILED', scrubDetail(e.message))); }
    return report;
  };

  HamiClient.VERSION = VERSION;
  HamiClient.LAYERS = LAYERS;
  HamiClient.scrubDetail = scrubDetail;
  HamiClient.HamiError = HamiError;
  global.HamiClient = HamiClient;
  if (typeof module !== 'undefined' && module.exports) module.exports = HamiClient;
})(typeof window !== 'undefined' ? window : globalThis);

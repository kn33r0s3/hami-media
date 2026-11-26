// Hami browser SDK — Part 4: browser streamer.
//
// Browser WebRTC APIs only: RTCPeerConnection, getUserMedia,
// RTCRtpSender.replaceTrack / setParameters for layer switching.
// No dependencies, no build step.
//
// S2: Sets codec preferences to H.264 profile 42001f before createOffer.
// S3: Points at /publisher and /viewer (not /api/publish or /api/subscribe).
// S3: Removes reportError (D12: its endpoint does not exist).

(function (global) {
  'use strict';

  var VERSION = '0.3.1';
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

  // setH264CodecPreference sets codec preferences to H.264 profile 42001f
  // on the video transceiver before createOffer (S2, D13).
  function setH264CodecPreference(transceiver) {
    if (!transceiver || !transceiver.setCodecPreferences) return;
    if (!RTCRtpSender || !RTCRtpSender.getCapabilities) return;
    var caps = RTCRtpSender.getCapabilities('video');
    if (!caps || !caps.codecs) return;
    var h264Codecs = caps.codecs.filter(function (c) {
      return c.mimeType === 'video/H264' &&
             c.sdpFmtpLine &&
             c.sdpFmtpLine.indexOf('profile-level-id=42001f') >= 0;
    });
    if (h264Codecs.length === 0) {
      console.warn('HamiSDK: No H.264 42001f codec found in browser capabilities');
      return;
    }
    try {
      transceiver.setCodecPreferences(h264Codecs);
    } catch (e) {
      console.warn('HamiSDK: setCodecPreferences failed:', e.message);
    }
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
    this.publisherID = opts.publisherID || '';
    this.viewerID = opts.viewerID || '';
    this.subscriberID = opts.subscriberID || this.viewerID;
    this.displayName = opts.displayName || '';
    this.pc = null;
    this.publishPC = null; // separate PC for publishing
    this.layer = 'top';
    this.onTrack = opts.onTrack || null;
    this.onError = opts.onError || null;
    this.onMemberJoined = opts.onMemberJoined || null;
    this.onMemberLeft = opts.onMemberLeft || null;
    this.onChat = opts.onChat || null;
    this._pcFactory = opts.pcFactory || function (config) { return new RTCPeerConnection(config); };
    this._fetch = opts.fetchImpl || fetch.bind(global);
    this._getUserMedia = opts.getUserMedia || (navigator.mediaDevices && navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices));
    this._eventSource = null;
    this._localStream = null;
    this._cameraTrack = null;
    this._audioMuted = false;
    this._videoMuted = false;
    this._state = 'idle'; // idle, watching, live, left
  }

  HamiClient.prototype._post = async function (path, body) {
    var res;
    try {
      res = await this._fetch(this.serverUrl + path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
    } catch (e) {
      throw HamiError('NETWORK_ERROR', scrubDetail(e.message));
    }
    if (res.status === 401) throw HamiError('AUTH_ERROR', 'invalid or expired token');
    if (!res.ok) {
      var errText = '';
      try { var errBody = await res.json(); errText = errBody.error || ''; } catch (_) {}
      throw HamiError('SERVER_ERROR', 'POST ' + path + ' failed: ' + res.status + ' ' + errText);
    }
    return res.json();
  };

  // publish() creates a PeerConnection and negotiates a publish
  // offer (sendonly). The token must grant "publish" role.
  HamiClient.prototype.publish = async function (stream) {
    if (!stream) throw HamiError('INVALID_CONFIG', 'a MediaStream is required');
    this.publishPC = this._pcFactory({ iceServers: [] });
    var self = this;
    this._localStream = stream;

    stream.getTracks().forEach(function (track) {
      if (track.kind === 'audio') track.enabled = !self._audioMuted;
      if (track.kind === 'video') track.enabled = !self._videoMuted;
      var transceiver = self.publishPC.addTransceiver(track, { direction: 'sendonly' });
      // S2: set H.264 42001f preference on video transceivers
      if (track.kind === 'video') {
        setH264CodecPreference(transceiver);
        self._cameraTrack = track;
      }
    });

    var offer = await this.publishPC.createOffer();
    await this.publishPC.setLocalDescription(offer);
    var ans = await this._post('/publisher', {
      publisher_id: this.publisherID,
      offer: offer.sdp,
      token: this.token,
      display_name: this.displayName
    });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'publish answer missing');
    await this.publishPC.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    this._state = 'live';
    return { published: stream.getTracks().length };
  };

  HamiClient.prototype.replaceVideoTrack = async function (track) {
    if (!track || track.kind !== 'video') {
      throw HamiError('INVALID_TRACK', 'a video MediaStreamTrack is required');
    }
    if (!this.publishPC) throw HamiError('NOT_PUBLISHING', 'publish first');
    var sender = this.publishPC.getSenders().find(function (candidate) {
      return candidate.track && candidate.track.kind === 'video';
    });
    if (!sender) throw HamiError('NO_VIDEO_SENDER', 'no video sender found');
    track.enabled = !this._videoMuted;
    await sender.replaceTrack(track);
    this._cameraTrack = track;
    return { replaced: true };
  };

  // subscribe() creates a PeerConnection and negotiates a subscribe
  // offer (recvonly). The token must grant "subscribe" role.
  HamiClient.prototype.subscribe = async function (publisherID, trackID) {
    this.pc = this._pcFactory({ iceServers: [] });
    var self = this;
    this.pc.ontrack = function (ev) { if (self.onTrack) self.onTrack(ev.track, ev.streams); };
    this.pc.addTransceiver('video', { direction: 'recvonly' });
    this.pc.addTransceiver('audio', { direction: 'recvonly' });
    var offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    var ans = await this._post('/viewer', {
      viewer_id: this.viewerID,
      subscriber_id: this.subscriberID,
      key: { publisher_id: publisherID, track_id: trackID },
      offer: offer.sdp,
      token: this.token
    });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'subscribe answer missing');
    await this.pc.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    this._state = 'watching';
    return { room: this.room, layer: this.layer };
  };

  // For backward compatibility: join() now calls subscribe() on a default track.
  HamiClient.prototype.join = async function () {
    // Legacy path — not used by the new streamer/viewer pages.
    if (this.pc) throw HamiError('ALREADY_JOINED', 'already joined room ' + this.room);
    this.pc = this._pcFactory({ iceServers: [] });
    var self = this;
    this.pc.ontrack = function (ev) { if (self.onTrack) self.onTrack(ev.track, ev.streams); };
    this.pc.addTransceiver('video', { direction: 'recvonly' });
    this.pc.addTransceiver('audio', { direction: 'recvonly' });
    var offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    var ans = await this._post('/viewer', {
      viewer_id: this.viewerID || 'viewer-' + Date.now(),
      key: { publisher_id: 'default', track_id: 'video-0' },
      offer: offer.sdp,
      token: this.token
    });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'subscribe answer missing');
    await this.pc.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    this._state = 'watching';
    return { room: this.room, layer: this.layer };
  };

  // S5: mute/unmute toggles track.enabled without renegotiation.
  HamiClient.prototype.muteAudio = function (muted) {
    this._audioMuted = Boolean(muted);
    if (!this.publishPC) return;
    this.publishPC.getSenders().forEach(function (s) {
      if (s.track && s.track.kind === 'audio') s.track.enabled = !this._audioMuted;
    }, this);
  };

  HamiClient.prototype.muteVideo = function (muted) {
    this._videoMuted = Boolean(muted);
    if (!this.publishPC) return;
    this.publishPC.getSenders().forEach(function (s) {
      if (s.track && s.track.kind === 'video') s.track.enabled = !this._videoMuted;
    }, this);
  };

  // S6: screen share with replaceTrack.
  HamiClient.prototype.shareScreen = async function () {
    if (!this.publishPC) throw HamiError('NOT_PUBLISHING', 'publish first');
    var screenStream = await navigator.mediaDevices.getDisplayMedia({ video: true });
    var screenTrack = screenStream.getVideoTracks()[0];
    screenTrack.enabled = !this._videoMuted;
    var sender = this.publishPC.getSenders().find(function (s) {
      return s.track && s.track.kind === 'video';
    });
    if (!sender) throw HamiError('NO_VIDEO_SENDER', 'no video sender found');
    await sender.replaceTrack(screenTrack);
    var self = this;
    screenTrack.onended = function () {
      // Return to camera when screen share stops
      if (self._cameraTrack) {
        sender.replaceTrack(self._cameraTrack);
      }
    };
    return { sharing: true };
  };

  HamiClient.prototype.stopScreenShare = async function () {
    if (!this.publishPC || !this._cameraTrack) return;
    var sender = this.publishPC.getSenders().find(function (s) {
      return s.track && s.track.kind === 'video';
    });
    if (sender) {
      this._cameraTrack.enabled = !this._videoMuted;
      await sender.replaceTrack(this._cameraTrack);
    }
  };

  // S7: stop going live without leaving the room.
  HamiClient.prototype.stopPublishing = async function () {
    if (!this.publishPC) return;
    await this._post('/publisher/leave', {
      publisher_id: this.publisherID,
      token: this.token
    });
    try { this.publishPC.close(); } catch (e) { /* already closed */ }
    this.publishPC = null;
    this._state = this.pc ? 'watching' : 'idle';
  };

  // S8: connect to the SSE event stream for presence and chat.
  HamiClient.prototype.connectEvents = function (room) {
    var self = this;
    var url = this.serverUrl + '/rooms/' + encodeURIComponent(room || this.room) +
              '/events?token=' + encodeURIComponent(this.token);
    this._eventSource = new EventSource(url);

    this._eventSource.addEventListener('publisher-joined', function (ev) {
      var data = JSON.parse(ev.data);
      if (self.onMemberJoined) self.onMemberJoined(data);
    });
    this._eventSource.addEventListener('publisher-left', function (ev) {
      var data = JSON.parse(ev.data);
      if (self.onMemberLeft) self.onMemberLeft(data);
    });
    this._eventSource.addEventListener('chat', function (ev) {
      var data = JSON.parse(ev.data);
      if (self.onChat) self.onChat(data);
    });
    this._eventSource.onerror = function () {
      if (self.onError) self.onError(HamiError('SSE_ERROR', 'event stream disconnected'));
    };
  };

  // S10: send a chat message.
  HamiClient.prototype.sendChat = async function (text) {
    return this._post('/rooms/' + encodeURIComponent(this.room) + '/chat', {
      token: this.token,
      text: text,
      from: this.displayName || this.publisherID || this.viewerID || 'anonymous'
    });
  };

  // S8: get the member list.
  HamiClient.prototype.getMembers = async function (room) {
    var url = this.serverUrl + '/rooms/' + encodeURIComponent(room || this.room) +
              '/members?token=' + encodeURIComponent(this.token);
    var res = await this._fetch(url);
    if (!res.ok) throw HamiError('SERVER_ERROR', 'GET members failed: ' + res.status);
    return res.json();
  };

  // switchLayer() re-negotiates the subscription at a new layer.
  HamiClient.prototype.switchLayer = async function (layer) {
    if (LAYERS.indexOf(layer) === -1) throw HamiError('INVALID_LAYER', 'layer must be one of ' + LAYERS.join(', '));
    if (!this.pc) throw HamiError('NOT_JOINED', 'subscribe() first');
    var started = Date.now();
    var offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    var ans = await this._post('/viewer', {
      viewer_id: this.viewerID,
      key: { publisher_id: 'default', track_id: 'video-0' },
      offer: offer.sdp,
      token: this.token,
      layer: layer
    });
    if (!ans.answer) throw HamiError('SERVER_ERROR', 'layer-switch answer missing');
    await this.pc.setRemoteDescription({ type: 'answer', sdp: ans.answer });
    this.layer = layer;
    return { layer: layer, switchMs: Date.now() - started };
  };

  // S11: state accessor.
  HamiClient.prototype.getState = function () {
    return this._state;
  };

  // leave closes all connections and event streams.
  HamiClient.prototype.leave = async function () {
    if (this.publishPC) {
      try {
        await this._post('/publisher/leave', {
          publisher_id: this.publisherID,
          token: this.token
        });
      } catch (e) { /* best effort */ }
      try { this.publishPC.close(); } catch (e) { /* already closed */ }
      this.publishPC = null;
    }
    if (this.pc) { try { this.pc.close(); } catch (e) { /* already closed */ } this.pc = null; }
    if (this._eventSource) { this._eventSource.close(); this._eventSource = null; }
    this._state = 'left';
  };

  HamiClient.VERSION = VERSION;
  HamiClient.LAYERS = LAYERS;
  HamiClient.scrubDetail = scrubDetail;
  HamiClient.HamiError = HamiError;
  HamiClient.setH264CodecPreference = setH264CodecPreference;
  global.HamiClient = HamiClient;
  if (typeof module !== 'undefined' && module.exports) module.exports = HamiClient;
})(typeof window !== 'undefined' ? window : globalThis);

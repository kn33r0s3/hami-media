# Cluster D — Section 10 clients: status

Current implementation caveat: the mocked Web SDK tests do not establish
server compatibility. The prototype calls routes not implemented by the
internal loopback API; see the current [SDK status](../../../sdk.md).

## 10.1 Web SDK — PARTIAL
- SDK: `sdk/web/hami.js` (0.1.0, no dependencies, browser WebRTC APIs).
- Automated test: `test/web-sdk-test.mjs` — joins, publishes,
  subscribes (ontrack delivery), switches base/middle/top, reports a
  scrubbed error, leaves. Evidence: `log (sha256 1fb61a149820, 2 lines; kept outside repo)`.
- Gap: the harness stubs RTCPeerConnection/fetch/getUserMedia. There
  is no browser in the VM (no Chromium/Firefox, no Playwright
  cache), so the headless-browser test the Done line asks for cannot
  run here. 10.1 becomes DONE only when the same scenario passes in
  a real browser (Chrome/Safari/Firefox) against a live server.

## 10.2 iOS SDK — BLOCKED
No physical iOS device. No Swift SDK written (writing one without a
device to run it on would be unverified code).

## 10.3 Android SDK — BLOCKED
No physical Android device. Same reason as 10.2.

## 10.4 Playout tests — PARTIAL
- Web part DONE: `test/playout-test.mjs` — clean 30 fps stream
  reports 0 stalls; impaired stream (two late bursts) reports 2
  stall events / 20 late frames. Evidence:
  `log (sha256 24ae0d12414e, 2 lines; kept outside repo)`.
- iOS/Android parts BLOCKED with 10.2/10.3.

## 10.5 Device matrix — BLOCKED
Needs older Android, recent iPhone, mid-range Android, Chrome,
Safari, Firefox on hardware, plus 10 minutes of encoding with
thermal state. No devices available.

## 10.6 Client error reports — DONE (web)
- Report shape + validation + leak scan: `client_errors.go`,
  `validateClientErrorReport`, `scrubClientDetail`.
- Sample reports pass the leak scan; reports carrying tokens,
  emails, SDP or ICE candidates are rejected (test:
  `TestClusterDClientErrorReports`).
- SDK side: `HamiClient.reportError()` scrubs before sending.

## 10.7 Sample app — PARTIAL
- Web sample DONE: `sample-apps/web/` — runs from a fresh checkout
  with `python3 -m http.server`, no build step. Verified by serving
  it (see evidence log `log (sha256 617b11162909, 4 lines; kept outside repo)`).
- iOS/Android samples BLOCKED with 10.2/10.3.

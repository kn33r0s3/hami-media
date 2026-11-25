// Playout test, web part (job 10.4).
//
// A playout stall is a gap in rendered frames: the test feeds a
// jitter-buffer model with frame arrival times and counts stalls
// (a rendered frame arriving later than its playout deadline).
// Each platform reports playout stalls in its test output; this is
// the web part. iOS/Android parts stay BLOCKED (no devices).
//
// Model: 30 fps, playout deadline = scheduled render time + 50 ms
// jitter buffer. A frame is a stall if it arrives after its
// deadline; consecutive late frames count as one stall event.

export function countPlayoutStalls(arrivalsMs, fps = 30, jitterBufferMs = 50) {
  const interval = 1000 / fps;
  let stalls = 0;
  let stalled = false;
  let lateFrames = 0;
  for (let i = 0; i < arrivalsMs.length; i++) {
    const deadline = i * interval + jitterBufferMs;
    if (arrivalsMs[i] > deadline) {
      lateFrames++;
      if (!stalled) { stalls++; stalled = true; }
    } else {
      stalled = false;
    }
  }
  return { stalls, lateFrames, frames: arrivalsMs.length };
}

if (import.meta.url === `file://${process.argv[1]}`) {
  // Clean 30 fps stream: 300 frames on time -> 0 stalls.
  const clean = Array.from({ length: 300 }, (_, i) => i * (1000 / 30));
  const cleanRes = countPlayoutStalls(clean);
  // Impaired stream: a 400 ms burst at frames 100-109 and a 200 ms
  // burst at frames 200-209 (player re-buffers after each burst, so
  // each burst is one stall event).
  const impaired = clean.slice();
  for (let i = 100; i < 110; i++) impaired[i] += 400;
  for (let i = 200; i < 210; i++) impaired[i] += 200;
  const impRes = countPlayoutStalls(impaired);
  console.log(JSON.stringify({ platform: 'web', clean: cleanRes, impaired: impRes }));
  if (cleanRes.stalls !== 0) { console.error('PLAYOUT-TEST FAIL: clean stream stalled'); process.exit(1); }
  if (impRes.stalls !== 2) { console.error('PLAYOUT-TEST FAIL: expected 2 stall events, got ' + impRes.stalls); process.exit(1); }
  console.log('PLAYOUT-TEST PASS: web reports playout stalls (clean 0, impaired 2 stall events, ' + impRes.lateFrames + ' late frames)');
}

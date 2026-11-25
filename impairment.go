// Cluster C §7.5 — impairment matrix in the VM.
//
// The VM has no netem, so impairments are applied IN-PROCESS by an
// impairmentPipe that sits between sender and receiver in tests: loss
// (drop with probability p, seeded for determinism), fixed delay,
// jitter (extra random delay), reorder (hold-and-release), duplication.
// The exact settings are saved with each result (job 7.5 Done line).
// Deterministic seeds (math/rand with a fixed seed per trial) make
// repeated runs identical.
package main

import (
	"fmt"
	"math/rand"
	"time"
)

// impairmentSettings is one cell of the matrix. Saved verbatim with
// every result.
type impairmentSettings struct {
	LossPct   float64 `json:"loss_pct"`  // 0, 1, 3
	DelayMs   int     `json:"delay_ms"`  // 30, 100, 150
	JitterMs  int     `json:"jitter_ms"` // extra random delay 0..JitterMs
	Reorder   bool    `json:"reorder"`   // hold 1 packet, release out of order
	Duplicate bool    `json:"duplicate"` // deliver every packet twice
	Seed      int64   `json:"seed"`
}

// impairmentResult is the measured outcome of one cell.
type impairmentResult struct {
	Settings   impairmentSettings `json:"settings"`
	Sent       int                `json:"sent"`
	Received   int                `json:"received"`
	Lost       int                `json:"lost"`
	Duplicated int                `json:"duplicated"`
	MaxDelayMs int                `json:"max_delay_ms"`
	OutOfOrder int                `json:"out_of_order"`
}

// impairmentPipe applies settings deterministically to a stream of
// numbered packets. Deliveries are (seq, deliverAt) pairs; the caller
// advances a virtual clock, so no real sleeping is needed.
type impairmentPipe struct {
	settings impairmentSettings
	rng      *rand.Rand
	held     *int // one held packet for reorder
	heldAt   time.Time
	heldSent time.Time
}

func newImpairmentPipe(s impairmentSettings) *impairmentPipe {
	return &impairmentPipe{settings: s, rng: rand.New(rand.NewSource(s.Seed))}
}

// send processes one packet sent at now. Returns deliveries (possibly
// zero on loss, two on duplication, delayed by delay+jitter).
func (p *impairmentPipe) send(seq int, now time.Time) []impairmentDelivery {
	if p.rng.Float64()*100 < p.settings.LossPct {
		return nil
	}
	delay := time.Duration(p.settings.DelayMs) * time.Millisecond
	if p.settings.JitterMs > 0 {
		delay += time.Duration(p.rng.Intn(p.settings.JitterMs+1)) * time.Millisecond
	}
	out := []impairmentDelivery{}
	emit := func(s int, at time.Time) {
		out = append(out, impairmentDelivery{Seq: s, SentAt: now, DeliverAt: at})
		if p.settings.Duplicate {
			out = append(out, impairmentDelivery{Seq: s, SentAt: now, DeliverAt: at, IsDuplicate: true})
		}
	}
	if p.settings.Reorder {
		if p.held != nil {
			held := *p.held
			heldAt := p.heldAt
			heldSent := p.heldSent
			p.held = nil
			// Release the held (earlier) packet AFTER the current one:
			// emit current first, held second => out of order.
			emit(seq, now.Add(delay))
			out = append(out, impairmentDelivery{Seq: held, SentAt: heldSent, DeliverAt: heldAt.Add(time.Millisecond)})
			if p.settings.Duplicate {
				out = append(out, impairmentDelivery{Seq: held, SentAt: heldSent, DeliverAt: heldAt.Add(time.Millisecond), IsDuplicate: true})
			}
			return out
		}
		h := seq
		p.held = &h
		p.heldAt = now.Add(delay)
		p.heldSent = now
		return nil
	}
	emit(seq, now.Add(delay))
	return out
}

// flush releases a held reorder packet at the end of a run.
func (p *impairmentPipe) flush() []impairmentDelivery {
	if p.held == nil {
		return nil
	}
	seq := *p.held
	p.held = nil
	out := []impairmentDelivery{{Seq: seq, SentAt: p.heldSent, DeliverAt: p.heldAt}}
	if p.settings.Duplicate {
		out = append(out, impairmentDelivery{Seq: seq, SentAt: p.heldSent, DeliverAt: p.heldAt, IsDuplicate: true})
	}
	return out
}

type impairmentDelivery struct {
	Seq         int
	SentAt      time.Time
	DeliverAt   time.Time
	IsDuplicate bool
}

// runImpairmentMatrix runs the §7.5 matrix: loss 0/1/3% x delay
// 30/100/150 ms, plus jitter, reorder and duplication cells, 1000
// packets per cell, fixed seeds. Returns one result per cell.
func runImpairmentMatrix() []impairmentResult {
	var results []impairmentResult
	now := time.Now()
	cells := []impairmentSettings{}
	for _, loss := range []float64{0, 1, 3} {
		for _, delay := range []int{30, 100, 150} {
			cells = append(cells, impairmentSettings{LossPct: loss, DelayMs: delay, Seed: 42})
		}
	}
	cells = append(cells,
		impairmentSettings{LossPct: 1, DelayMs: 100, JitterMs: 20, Seed: 42},
		impairmentSettings{LossPct: 0, DelayMs: 30, Reorder: true, Seed: 42},
		impairmentSettings{LossPct: 0, DelayMs: 30, Duplicate: true, Seed: 42},
	)
	for _, cell := range cells {
		results = append(results, runImpairmentCell(cell, 1000, now))
	}
	return results
}

func runImpairmentCell(s impairmentSettings, n int, start time.Time) impairmentResult {
	pipe := newImpairmentPipe(s)
	res := impairmentResult{Settings: s, Sent: n}
	var deliveries []impairmentDelivery
	for i := 0; i < n; i++ {
		deliveries = append(deliveries, pipe.send(i, start.Add(time.Duration(i)*20*time.Millisecond))...)
	}
	deliveries = append(deliveries, pipe.flush()...)
	// Deliveries are already in emission order (the order a receiver
	// sees). Do NOT sort by DeliverAt: sorting would hide reorder.
	seen := map[int]bool{}
	lastSeq := -1
	for _, d := range deliveries {
		if d.IsDuplicate {
			res.Duplicated++
			continue
		}
		if seen[d.Seq] {
			res.Duplicated++
			continue
		}
		seen[d.Seq] = true
		res.Received++
		if d.Seq < lastSeq {
			res.OutOfOrder++
		}
		if d.Seq > lastSeq {
			lastSeq = d.Seq
		}
		if ms := int(d.DeliverAt.Sub(d.SentAt) / time.Millisecond); ms > res.MaxDelayMs {
			res.MaxDelayMs = ms
		}
	}
	res.Lost = n - res.Received
	return res
}

// describeImpairmentCell renders one result line for logs.
func describeImpairmentCell(r impairmentResult) string {
	return fmt.Sprintf("loss=%.0f%% delay=%dms jitter=%dms reorder=%v dup=%v seed=%d: sent=%d received=%d lost=%d dup=%d ooo=%d maxDelay=%dms",
		r.Settings.LossPct, r.Settings.DelayMs, r.Settings.JitterMs, r.Settings.Reorder, r.Settings.Duplicate,
		r.Settings.Seed, r.Sent, r.Received, r.Lost, r.Duplicated, r.OutOfOrder, r.MaxDelayMs)
}

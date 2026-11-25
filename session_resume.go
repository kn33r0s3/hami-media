// Cluster D job 14.9: crash restart behaviour.
//
// After a process restart, clients reconnect automatically and
// rejoin their rooms. The client side is a reconnect loop with
// bounded backoff; the server side is the existing join path (the
// client re-joins with the same room + token, and session resume
// restores its state inside the agreed window — see 9.6).
//
// This file is the client-side reconnect loop, tested in-process by
// simulating a restart: the join function fails while the server is
// "down" and succeeds once it is back. The test asserts the client
// rejoins the same room automatically, within the attempt budget.

package main

import (
	"errors"
	"time"
)

// clusterDReconnectClient reconnects and rejoins a room after a
// server restart.
type clusterDReconnectClient struct {
	RoomID      string
	MaxAttempts int
	Backoff     time.Duration
	// join is the join attempt; it fails while the server is down.
	join func(roomID string) error
	// sleep is injectable for tests (defaults to time.Sleep).
	sleep    func(time.Duration)
	Attempts int
	Rejoined bool
	RejoinAt time.Time
}

// reconnect runs the loop until join succeeds or attempts run out.
func (c *clusterDReconnectClient) reconnect() error {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.Backoff <= 0 {
		c.Backoff = 500 * time.Millisecond
	}
	sleep := c.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	var lastErr error
	for attempt := 1; attempt <= c.MaxAttempts; attempt++ {
		c.Attempts = attempt
		if err := c.join(c.RoomID); err == nil {
			c.Rejoined = true
			c.RejoinAt = time.Now()
			return nil
		} else {
			lastErr = err
		}
		if attempt < c.MaxAttempts {
			sleep(c.Backoff)
		}
	}
	return errors.New("reconnect: server did not come back within the attempt budget: " + lastErr.Error())
}

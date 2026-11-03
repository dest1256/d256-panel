package service

// The bulk ops walked their inbounds one node round-trip at a time, so a client
// spanning several nodes cost the SUM of every node's latency. Each test below
// times out on the barrier unless the pushes overlap.

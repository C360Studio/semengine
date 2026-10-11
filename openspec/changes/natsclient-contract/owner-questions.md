# Owner questions for PR #177, design revision 3

No question is open. This file records the answer to revision 2's question, and what the design did with it.

## Q1. Should natsclient shorten nats.go's one-minute write timeout? (answered)

**Your answer** (#170 comment 6110309758), with spelling corrected: "i would lean no as well, but i also think it
should be something we can control since i do not want tests waiting for a minute just to test native timeout
behavior."

**What the design does with it** (design D4):

- **The defaults stay as they are.** nats.go's write timeout is one minute, and natsclient's PING interval is 30 s.
  The write timeout is how long nats.go lets one write wait for the server to read it. The PING interval is how often
  nats.go checks that the server still answers.
- **natsclient gains two settings:**
  - `WithWriteTimeout(d)` sets the write timeout;
  - `WithPingInterval(d)` sets the PING interval.

  Each refuses zero or a negative value, because nats.go reads zero as "never time out" or "never check".
- **Why the two go together.** The probes found that a short write timeout alone is not enough. When it cuts a write
  halfway, the connection can stay "connected" while carrying nothing, until the PING check notices (P22, P24: 6 of
  6 runs stayed stuck for 5 s). With both at 300 ms, the connection recovered 3 ms after the server read again (P24,
  3 of 3). The write timeout's documentation says to set both.
- **Why two settings rather than one.** Each matches one nats.go setting, as natsclient's other options already do. A
  short PING interval is also useful on its own, to notice sooner that an idle server has gone away. One setting
  that took two durations would be easy to call with them swapped.
- **The tests use the settings,** so no test waits a minute. The settings' own test uses 300 ms.

## A choice the design made without asking

**Ending a reconnect dial uses `net.Dialer.Cancel`, a field Go marks deprecated** (design D1).

The reviewer found that `Close` could still be held while nats.go dials a reconnect. A probe measured 4.69 s (P23).
The fix closes a channel that the dial watches; with it, `Close` returns in 51 ms. "Deprecated" here means Go
recommends a newer way, `DialContext`. Go still supports the field, and will keep doing so.

The newer way needs a context: Go's standard value that carries a deadline and a cancel signal. nats.go's dialer
hands natsclient none. Our rules forbid both workarounds:

- keeping the caller's context, which `TestNoRetainedContext` checks;
- making a fresh one inside the dialer.

So the choice is between the deprecated field and leaving `Close` waiting up to 5 s per address. The field is
smaller, and it also closes the same gap in `Connect`. A test (T8) fails if Go ever stops honouring it. It is not
close enough to ask you. Say so if you see it otherwise.

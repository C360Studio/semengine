# Inventory evidence (local paths named are local only)

- `pin-race.txt`, `pin-repeat.txt`: the pin probe outputs cited in `inventory.md` § 0, run on
  `/tmp/gcpin/src` (tarball of SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`):
  `go test -count=1 -race` and `go test -count=5 -cpu=1 -shuffle=on` over
  `./processor/graph-clustering/ ./graph/structural/ ./graph/inference/`. Both `rc=0`.
- Not run: the pin's six `//go:build integration` files and the lifecycle suite (inventory § 0).
- Not yet run: `TestWatchOrderAcrossKeys` (#525) is task 2.3's probe; its observed order goes in `notes/2.3.md` as
  an observation at the pinned versions, not a promise.

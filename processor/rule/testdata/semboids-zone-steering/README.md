# semboids zone-steering rules

A rule pack that uses only built-in action types: each rule publishes a steering message when a boid enters or leaves
a zone. `TestSemboidsPackLoads` in `load_test.go` decodes every rule in these files and loads it with the handler set
`NewHandlers(nil)` builds, so a change to the built-in checks that would refuse a product's working pack fails there.

The files are copied unchanged from the semboids repository, <https://github.com/C360Studio/semboids>, at commit
`ab4e0fbc1b382c4fda10bae988662ae0e0bc1e5c`, from `configs/rules/zone-steering/`:

| File | What it holds |
| --- | --- |
| `food.json` | Two rules: attract a boid into a food zone, and cancel that on exit |
| `predator.json` | Three rules: steer a boid away from a predator zone, and cancel that on exit |
| `wind.json` | Two rules: push a boid while it is in a wind zone, and cancel that on exit |

Each file holds a JSON array of rules. Every condition reads a `$message.*` field, so no predicate needs declaring.

semboids is MIT licensed (its `LICENSE` file at that commit, copyright 2026 Complete 360 Studio). Do not edit the
files: the test proves that this pack, as the product ships it, loads.

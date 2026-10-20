# Probe: which listener pairs can both bind (#328)

Run 2026-10-20 outside the repository. macOS 26.5.2 darwin/arm64, Go 1.26.9 (`go run .`). Linux
7.0.14-linuxkit arm64 in `golang:1.26.9` (`sha256:37cfcca63222ec499bec5e8ad7aad52dbf7529db36d5812b8eae2f0621aa92d6`),
`docker run --rm --name semengine-probe-328 -v $PWD:/src:ro -w /src -e GOCACHE=/tmp/gc -e GOFLAGS=-buildvcs=false
golang:1.26.9 go run .`. Each pair binds the first, then the second, on a port found free on both transports, with
`net.Listen("tcp", …)` / `net.ListenPacket("udp", …)`.

```go
package main

import (
    "fmt"
    "net"
    "strconv"
)

type closer interface{ Close() error }

func bind(network, host string, port int) (closer, string, error) {
    addr := net.JoinHostPort(host, strconv.Itoa(port))
    if network == "tcp" {
        l, err := net.Listen("tcp", addr)
        if err != nil {
            return nil, "", err
        }
        return l, l.Addr().String(), nil
    }
    c, err := net.ListenPacket("udp", addr)
    if err != nil {
        return nil, "", err
    }
    return c, c.LocalAddr().String(), nil
}

func freePort() int {
    for {
        l, _ := net.Listen("tcp", ":0")
        p := l.Addr().(*net.TCPAddr).Port
        l.Close()
        ok := true
        for _, n := range []string{"tcp", "udp"} {
            c, _, err := bind(n, "", p)
            if err != nil {
                ok = false
                break
            }
            c.Close()
        }
        if ok {
            return p
        }
    }
}

func main() {
    // Each entry: {network1, host1, network2, host2}. Every table row below is listed, in both orders where the
    // table says so. The last nine entries (mapped, long-form, upper-case and 127.0.0.2 hosts) were first run as
    // two smaller programs with these same bind calls; they are merged here so one program re-runs the table.
    var pairs [][4]string
    for _, n := range []string{"tcp", "udp"} {
        for _, h := range [][2]string{
            {"0.0.0.0", "127.0.0.1"}, {"127.0.0.1", "0.0.0.0"},
            {"0.0.0.0", "0.0.0.0"}, {"127.0.0.1", "127.0.0.1"},
            {"::", "0.0.0.0"}, {"0.0.0.0", "::"},
            {"::1", "127.0.0.1"}, {"127.0.0.1", "::1"},
            {"::", "::1"}, {"::1", "::"},
            {"::", "127.0.0.1"}, {"127.0.0.1", "::"},
            {"0.0.0.0", "::1"}, {"::1", "0.0.0.0"},
            {"", "0.0.0.0"}, {"0.0.0.0", ""},
            {"", "127.0.0.1"}, {"127.0.0.1", ""},
            {"localhost", "127.0.0.1"}, {"127.0.0.1", "localhost"},
            {"localhost", "0.0.0.0"}, {"0.0.0.0", "localhost"},
            {"localhost", "::1"},
        } {
            pairs = append(pairs, [4]string{n, h[0], n, h[1]})
        }
    }
    pairs = append(pairs,
        [4]string{"tcp", "0.0.0.0", "udp", "0.0.0.0"},
        [4]string{"udp", "0.0.0.0", "tcp", "0.0.0.0"},
        [4]string{"tcp", "127.0.0.1", "udp", "127.0.0.1"},
        [4]string{"tcp", "::ffff:127.0.0.1", "tcp", "127.0.0.1"},
        [4]string{"udp", "::ffff:127.0.0.1", "udp", "127.0.0.1"},
        [4]string{"tcp", "0:0:0:0:0:0:0:1", "tcp", "::1"},
        [4]string{"tcp", "::ffff:0.0.0.0", "tcp", "127.0.0.1"},
        [4]string{"tcp", "LOCALHOST", "tcp", "127.0.0.1"},
        [4]string{"tcp", "127.0.0.1", "tcp", "127.0.0.2"}, // Linux only: macOS has no 127.0.0.2
        [4]string{"udp", "127.0.0.1", "udp", "127.0.0.2"}, // Linux only
    )
    for _, p := range pairs {
        port := freePort()
        c1, a1, err := bind(p[0], p[1], port)
        if err != nil {
            fmt.Println(p, "first failed:", err)
            continue
        }
        c2, a2, err := bind(p[2], p[3], port)
        fmt.Println(p, "first bound", a1, "second:", a2, err)
        if err == nil {
            c2.Close()
        }
        c1.Close()
    }
}
```

Go binds `0.0.0.0`, `::` and `""` as `[::]` (dual-stack) on both systems.

| Pair (same port, first then second; each also run reversed unless noted) | TCP macOS | TCP Linux | UDP macOS | UDP Linux |
| --- | --- | --- | --- | --- |
| `0.0.0.0` / `127.0.0.1` | both bind | second fails | second fails | second fails |
| same host twice (`0.0.0.0`, `127.0.0.1`) | second fails | second fails | second fails | second fails |
| `::` / `0.0.0.0`; `""` / `0.0.0.0` | second fails | second fails | second fails | second fails |
| `::1` / `127.0.0.1` | both bind | both bind | both bind | both bind |
| `::` / `::1`; `0.0.0.0` / `::1` | both bind | second fails | second fails | second fails |
| `::` / `127.0.0.1`; `""` / `127.0.0.1` | both bind | second fails | second fails | second fails |
| `localhost` / `127.0.0.1` (`localhost` bound `127.0.0.1`) | second fails | second fails | second fails | second fails |
| `localhost` / `0.0.0.0` | both bind | second fails | second fails | second fails |
| `localhost` / `::1` | both bind | both bind | both bind | both bind |
| tcp `0.0.0.0` / udp `0.0.0.0`; tcp `127.0.0.1` / udp `127.0.0.1` | both bind | both bind | (same rows) | (same rows) |
| `::ffff:127.0.0.1` / `127.0.0.1` (one order) | second fails | second fails | second fails | second fails |
| `0:0:0:0:0:0:0:1` / `::1` (one order) | second fails | second fails | not run | not run |
| `::ffff:0.0.0.0` / `127.0.0.1` (one order) | both bind | second fails | not run | not run |
| `LOCALHOST` / `127.0.0.1` (one order) | second fails | second fails | not run | not run |
| `127.0.0.1` / `127.0.0.2` (one order) | not run (no `127.0.0.2` on macOS) | both bind | not run | both bind |

Not probed, not promised: other OSes and kernels, other Go versions, listeners opened with `tcp4`/`tcp6`/`udp4`/
`udp6` or `SO_REUSEPORT`, and what a host name resolves to at `Start`.

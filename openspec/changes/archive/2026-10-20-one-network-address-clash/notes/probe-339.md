# Probe: do IPv6 zone spellings name one listener? (#339)

Run 2026-10-20 outside the repository with Go 1.26.9. On macOS 26.5.2 darwin/arm64 (`go run .`), `lo0` is index 1
and has `fe80::1` by default; `anpi2` (index 4) was the other interface. Linux 7.0.14-linuxkit arm64 ran in
`golang:1.26.9`:

```text
docker run --rm --name semengine-probe-339 --cap-add NET_ADMIN -v $PWD:/src:ro -w /src -e GOCACHE=/tmp/gc \
  -e GOFLAGS=-buildvcs=false golang:1.26.9 sh -c 'apt-get -qq update && apt-get -qq install -y iproute2; \
  ip -6 addr add fe80::1/64 dev lo; go run .'
```

There, `lo` is index 1 and `eth0` (index 11) has no IPv6. Each pair binds the first, then the second, on a port
found free on both transports, with `net.Listen("tcp", …)` / `net.ListenPacket("udp", …)`.

```go
// Probe for SemEngine #339: do IPv6 zone spellings name one listener?
package main

import (
    "fmt"
    "net"
    "runtime"
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
    fmt.Println("#", runtime.GOOS, runtime.GOARCH, runtime.Version())
    var lo, other net.Interface
    var linkLocal string
    var llIface net.Interface
    ifaces, _ := net.Interfaces()
    for _, ifc := range ifaces {
        addrs, _ := ifc.Addrs()
        for _, a := range addrs {
            ip := a.(*net.IPNet).IP
            if ip.To4() == nil && ip.IsLinkLocalUnicast() && (linkLocal == "" || ifc.Flags&net.FlagLoopback != 0) {
                linkLocal, llIface = ip.String(), ifc
            }
        }
        if ifc.Flags&net.FlagLoopback != 0 && lo.Name == "" {
            lo = ifc
        } else if ifc.Flags&net.FlagUp != 0 && other.Name == "" && ifc.Flags&net.FlagLoopback == 0 {
            other = ifc
        }
    }
    li := strconv.Itoa(lo.Index)
    oi := strconv.Itoa(other.Index)
    fmt.Printf("# loopback %s index %s; other %s index %s; link-local %q on %s index %d\n",
        lo.Name, li, other.Name, oi, linkLocal, llIface.Name, llIface.Index)
    hosts := [][2]string{
        {"::1", "::1%" + lo.Name}, {"::1%" + lo.Name, "::1"},
        {"::1", "::1%" + li}, {"::1%" + li, "::1"},
        {"::1%" + lo.Name, "::1%" + li}, {"::1%" + li, "::1%" + lo.Name},
        {"::1%" + lo.Name, "::1%" + lo.Name},
        {"::1", "::1%" + other.Name}, {"::1%" + other.Name, "::1"},
        {"::1%" + lo.Name, "::1%" + other.Name},
        {"::", "::1%" + lo.Name}, {"::1%" + lo.Name, "::"},
    }
    if linkLocal != "" {
        z, zi := linkLocal+"%"+llIface.Name, linkLocal+"%"+strconv.Itoa(llIface.Index)
        hosts = append(hosts,
            [2]string{linkLocal, z}, [2]string{z, linkLocal},
            [2]string{z, zi}, [2]string{zi, z},
            [2]string{z, z},
            [2]string{z, linkLocal + "%" + other.Name}, [2]string{linkLocal + "%" + other.Name, z},
            [2]string{"::", z}, [2]string{z, "::"},
            [2]string{z, "::1"},
        )
    }
    for _, n := range []string{"tcp", "udp"} {
        for _, h := range hosts {
            port := freePort()
            c1, a1, err := bind(n, h[0], port)
            if err != nil {
                fmt.Printf("| %s | %q | %q | FIRST FAILED: %v | - |\n", n, h[0], h[1], err)
                continue
            }
            c2, a2, err := bind(n, h[1], port)
            res := "both bind (" + a2 + ")"
            if err != nil {
                res = "second fails: " + err.Error()
            } else {
                c2.Close()
            }
            c1.Close()
            fmt.Printf("| %s | %q | %q | %s | %s |\n", n, h[0], h[1], a1, res)
        }
    }
}
```

`LO` is the loopback's name (`lo0` on macOS, `lo` on Linux); `1` is its index; `OTHER` is the other interface. "In
use" is `address already in use`. "Cannot assign" is `can't assign requested address` on macOS and `invalid
argument` (unzoned) or `cannot assign requested address` on Linux. Every row was run on TCP and UDP, and the results
match unless marked.

| First | Second | macOS | Linux |
| --- | --- | --- | --- |
| `::1` | `::1%LO`, `::1%1`, `::1%OTHER` (each order) | second fails: in use | second fails: in use |
| `::1%LO` | `::1%1`, `::1%LO`, `::1%OTHER` (each order for `%1`) | second fails: in use | second fails: in use |
| `::` | `::1%LO` (each order) | TCP: both bind; UDP: second fails: in use | second fails: in use |
| `fe80::1` (no zone) | `fe80::1%LO` | first fails: cannot assign | first fails: cannot assign |
| `fe80::1%LO` | `fe80::1` (no zone) | second fails: cannot assign | second fails: cannot assign |
| `fe80::1%LO` | `fe80::1%1` (each order) | second fails: in use | second fails: in use |
| `fe80::1%LO` | `fe80::1%LO` | second fails: in use | second fails: in use |
| `fe80::1%LO` | `fe80::1%OTHER` (each order) | the `%OTHER` bind fails: cannot assign | the `%OTHER` bind fails: cannot assign |
| `::` | `fe80::1%LO` (each order) | TCP: both bind; UDP: second fails: in use | second fails: in use |
| `fe80::1%LO` | `::1` | both bind | both bind |

Not probed, not promised: one link-local address on two interfaces (neither environment would add `fe80::1` to a
second interface: macOS has none by default, and Docker Desktop's `eth0` has IPv6 disabled), other OSes and kernels,
other Go versions.

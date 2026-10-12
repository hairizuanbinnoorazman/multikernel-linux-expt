// Diagnostic only: use the production authenticated agent packet interface.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"golang.org/x/sys/unix"
	"os"
	"strings"
	"syscall"
	"time"
)

func main() {
	socket := flag.String("socket", "", "private relay socket")
	generation := flag.String("generation", "", "generation")
	tokenFile := flag.String("token-file", "", "private credential file")
	flag.Parse()
	if err := run(*socket, *generation, *tokenFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(socket, generation, tokenFile string) error {
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		return err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return fmt.Errorf("invalid credential")
	}
	c, err := agent.Dial(socket, "runtime-g7-authority", generation, 7717, key)
	if err != nil {
		return err
	}
	defer c.Close()
	results := map[string]any{}
	defer func() { encoded, _ := json.MarshalIndent(results, "", "  "); fmt.Println(string(encoded)) }()
	call := func(method string, request any) error {
		var response any
		err := c.Call(method, request, &response)
		if err == nil {
			results[method] = response
		}
		return err
	}
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	req, err := unix.NewIfreq("mkonline0")
	if err != nil {
		return err
	}
	req.SetUint16(0x1001)
	if err = unix.IoctlIfreq(fd, 0x400454ca, req); err != nil {
		return err
	}
	if err = call("ConfigureNetwork", agent.NetworkConfig{Name: "mkn0", Address: "10.231.7.2/30", Gateway: "10.231.7.1", MTU: 1500, Nameservers: []string{"8.8.8.8"}}); err != nil {
		return err
	}
	if err = call("CreateProcess", map[string]any{"ID": "p1", "Bundle": "/bundle"}); err != nil {
		return err
	}
	if err = call("StartProcess", map[string]any{"ID": "p1"}); err != nil {
		return err
	}
	buf := make([]byte, 65535)
	var stdout, stderr []byte
	var so, se uint64
	var packetsIn, packetsOut uint64
	nextOutput := time.Now()
	deadline := time.Now().Add(20 * time.Minute)
	stopped := false
	for time.Now().Before(deadline) {
		n, e := unix.Read(fd, buf)
		if e != nil && e != syscall.EAGAIN && e != syscall.EWOULDBLOCK {
			return e
		}
		if e != nil {
			n = 0
		}
		if n > 1500 {
			return fmt.Errorf("oversized TUN packet %d", n)
		}
		var response struct {
			Packet []byte `json:"packet"`
		}
		if err = c.Call("ExchangeNetwork", map[string]any{"packet": buf[:n]}, &response); err != nil {
			return err
		}
		if n > 0 {
			packetsIn++
		}
		if len(response.Packet) > 0 {
			if _, err = unix.Write(fd, response.Packet); err != nil {
				return err
			}
			packetsOut++
		}
		if time.Now().After(nextOutput) {
			var out struct {
				Stdout, Stderr  []byte
				StdoutOffset    uint64 `json:"stdout_offset"`
				StderrOffset    uint64 `json:"stderr_offset"`
				Status          string
				StdoutTruncated bool `json:"stdout_truncated"`
				StderrTruncated bool `json:"stderr_truncated"`
			}
			if err = c.Call("ReadProcessOutput", map[string]any{"id": "p1", "stdout_offset": so, "stderr_offset": se, "limit": uint64(64 << 10)}, &out); err != nil {
				return err
			}
			if out.StdoutTruncated || out.StderrTruncated {
				return fmt.Errorf("probe output truncated")
			}
			fmt.Fprint(os.Stderr, string(out.Stdout), string(out.Stderr))
			stdout = append(stdout, out.Stdout...)
			stderr = append(stderr, out.Stderr...)
			so, se = out.StdoutOffset, out.StderrOffset
			if out.Status == "STOPPED" && len(out.Stdout) == 0 && len(out.Stderr) == 0 {
				stopped = true
				break
			}
			nextOutput = time.Now().Add(100 * time.Millisecond)
		}
	}
	if !stopped {
		return fmt.Errorf("online probe exceeded deadline")
	}
	if err = call("WaitProcess", map[string]any{"id": "p1"}); err != nil {
		return err
	}
	wait := results["WaitProcess"].(map[string]any)
	wait["stdout"] = string(stdout)
	wait["stderr"] = string(stderr)
	results["NetworkPacketCounts"] = map[string]uint64{"to_child": packetsIn, "from_child": packetsOut}
	for _, method := range []string{"DeleteProcess", "CloseNetwork", "Quiesce", "Shutdown"} {
		if err = call(method, map[string]any{"id": "p1"}); err != nil {
			return err
		}
	}
	return nil
}

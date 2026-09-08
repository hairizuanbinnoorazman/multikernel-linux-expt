//go:build linux

package network

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
)

type recordingRunner struct {
	calls  []string
	failAt int
	count  int
}

type inspectingRunner struct {
	recordingRunner
	corrupt bool
}

func (r *inspectingRunner) Output(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	if err := r.Run(ctx, name, arguments...); err != nil {
		return nil, err
	}
	if r.corrupt {
		return []byte("unexpected state\n"), nil
	}
	command := strings.Join(arguments, " ")
	switch {
	case strings.Contains(command, "-o link show dev mkv"):
		return []byte("2: mkv0123456789a: <UP> mtu 1400 state UP\n"), nil
	case strings.Contains(command, "route show 172.31.0.0/30"):
		return []byte("172.31.0.0/30 via 100.64.0.2 dev mkv0123456789a\n"), nil
	case strings.Contains(command, "-o address show dev eth0"):
		return []byte("inet 172.31.0.1/30 scope global eth0\n"), nil
	case strings.Contains(command, "-o link show dev eth0"):
		return []byte("3: eth0: <UP> mtu 1400 state UP\n"), nil
	case strings.Contains(command, "sysctl -n"):
		return []byte("1\n"), nil
	case strings.Contains(command, "route show default"):
		return []byte("default via 100.64.0.1 dev mkhost0\n"), nil
	default:
		return nil, nil
	}
}

func (r *recordingRunner) Run(_ context.Context, name string, arguments ...string) error {
	r.calls = append(r.calls, name+" "+strings.Join(arguments, " "))
	r.count++
	if r.failAt > 0 && r.count == r.failAt {
		return errors.New("injected command failure")
	}
	return nil
}

func linuxEndpoint() Endpoint {
	return Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0",
		NetNS: "/proc/123/ns/net", Generation: "0123456789abcdef0123456789abcdef",
		Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400,
	}
}

func TestLinuxBackendPolicyAndNamespaceCommands(t *testing.T) {
	runner := &recordingRunner{}
	backend := LinuxBackend{Runner: runner, IP: "ip", IPTables: "iptables", Nsenter: "nsenter", Egress: "ens4"}
	if err := backend.Add(context.Background(), linuxEndpoint()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, required := range []string{
		"ip link set mkhost0 netns 123",
		"nsenter --net=/proc/123/ns/net -- ip tuntap add dev eth0 mode tun",
		"iptables -w -A MK-0123456789ab ! -s 172.31.0.2/32 -j DROP",
		"iptables -w -t nat -A POSTROUTING -s 172.31.0.2/32 -o ens4 -j MASQUERADE",
		"iptables -w -A MK-0123456789ab -d 169.254.169.254/32 -p udp --dport 53 -j ACCEPT",
		"iptables -w -A MK-0123456789ab -d 169.254.169.254/32 -j REJECT",
		"iptables -w -A MK-0123456789ab -o mkv+ -j DROP",
		"iptables -w -A MK-0123456789ab -j DROP",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("missing command %q in:\n%s", required, joined)
		}
	}
}

func TestLinuxBackendRollsBackEveryAddBoundary(t *testing.T) {
	baseline := &recordingRunner{}
	backend := LinuxBackend{Runner: baseline, IP: "ip", IPTables: "iptables", Nsenter: "nsenter", Egress: "ens4"}
	if err := backend.Add(context.Background(), linuxEndpoint()); err != nil {
		t.Fatal(err)
	}
	addCount := len(baseline.calls)
	for boundary := 1; boundary <= addCount; boundary++ {
		runner := &recordingRunner{failAt: boundary}
		backend.Runner = runner
		if err := backend.Add(context.Background(), linuxEndpoint()); err == nil {
			t.Fatalf("boundary %d unexpectedly succeeded", boundary)
		}
		joined := strings.Join(runner.calls, "\n")
		for _, cleanup := range []string{
			"iptables -w -F MK-0123456789ab",
			"iptables -w -X MK-0123456789ab",
			"ip route del 172.31.0.0/30 via 100.64.0.2 dev mkv0123456789a",
			"ip link delete mkv0123456789a",
		} {
			if !strings.Contains(joined, cleanup) {
				t.Fatalf("boundary %d did not attempt cleanup %q", boundary, cleanup)
			}
		}
	}
}

func TestLinuxBackendRejectsMalformedIdentityBeforeCommands(t *testing.T) {
	for name, mutate := range map[string]func(*Endpoint){
		"short generation":   func(endpoint *Endpoint) { endpoint.Generation = "abcd" },
		"wrong host address": func(endpoint *Endpoint) { endpoint.Address = "172.31.0.1/30" },
		"wrong gateway":      func(endpoint *Endpoint) { endpoint.Gateway = "172.31.0.2" },
		"unsafe netns":       func(endpoint *Endpoint) { endpoint.NetNS = "/tmp/netns" },
	} {
		t.Run(name, func(t *testing.T) {
			value := linuxEndpoint()
			mutate(&value)
			runner := &recordingRunner{}
			backend := LinuxBackend{Runner: runner, Egress: "ens4"}
			if err := backend.Add(context.Background(), value); err == nil || len(runner.calls) != 0 {
				t.Fatalf("error=%v calls=%v", err, runner.calls)
			}
		})
	}
}

func TestLinuxBackendCheckValidatesObservedValues(t *testing.T) {
	runner := &inspectingRunner{}
	backend := LinuxBackend{Runner: runner, IP: "ip", IPTables: "iptables", Nsenter: "nsenter", Egress: "ens4"}
	if err := backend.Check(context.Background(), linuxEndpoint()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, required := range []string{
		"iptables -w -C MK-0123456789ab -o mkv+ -j DROP",
		"iptables -w -C MK-0123456789ab -j DROP",
		"iptables -w -C FORWARD -i ens4 -o mkv0123456789a -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
		"iptables -w -t nat -C POSTROUTING -s 172.31.0.2/32 -o ens4 -j MASQUERADE",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("CHECK omitted %q in:\n%s", required, joined)
		}
	}
	backend.Runner = &inspectingRunner{corrupt: true}
	if err := backend.Check(context.Background(), linuxEndpoint()); err == nil {
		t.Fatal("corrupt observed state passed CHECK")
	}
}

func TestResourceAbsentOnlyAcceptsKnownCommandDiagnostics(t *testing.T) {
	if !resourceAbsent(&commandError{err: errors.New("exit status 1"), output: "RTNETLINK answers: No such process"}) {
		t.Fatal("known absent route was not idempotent")
	}
	if resourceAbsent(errors.New("No such process")) || resourceAbsent(&commandError{err: errors.New("exit status 2"), output: "permission denied"}) {
		t.Fatal("unstructured or unexpected failure was ignored")
	}
}

func networkCommand(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecRunnerBoundsOutputDeadlineAndDescendants(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		runner := ExecRunner{Timeout: 50 * time.Millisecond, MaxOutput: 4096}
		started := time.Now()
		_, err := runner.Output(context.Background(), networkCommand(t, "sleep 60 & wait"))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("network command deadline error = %v", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("network command cancellation took %s", elapsed)
		}
	})

	t.Run("overflow and bounded diagnostic", func(t *testing.T) {
		runner := ExecRunner{Timeout: time.Second, MaxOutput: 32 << 10}
		_, err := runner.Output(context.Background(), networkCommand(t, "head -c 32769 /dev/zero"))
		if !errors.Is(err, boundedexec.ErrOutputLimit) {
			t.Fatalf("network command overflow error = %v", err)
		}
		var commandErr *commandError
		if !errors.As(err, &commandErr) || len(commandErr.output) > 16<<10 {
			t.Fatalf("network command diagnostic length = %d", len(commandErr.output))
		}
	})

	t.Run("combined output", func(t *testing.T) {
		runner := ExecRunner{Timeout: time.Second, MaxOutput: 4096}
		output, err := runner.Output(context.Background(), networkCommand(t, "printf stdout; printf stderr >&2"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(output), "stdout") || !strings.Contains(string(output), "stderr") {
			t.Fatalf("network combined output = %q", output)
		}
	})
}

func TestManagedNamespaceLifecycleIsGenerationNamed(t *testing.T) {
	runner := &recordingRunner{}
	backend := LinuxBackend{Runner: runner, IP: "ip"}
	path, err := backend.CreateNamespace(context.Background(), "0123456789abcdef0123456789abcdef")
	if err != nil || path != "/run/netns/mk-0123456789ab" {
		t.Fatalf("path=%q error=%v", path, err)
	}
	if err = backend.DeleteNamespace(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if strings.Join(runner.calls, "\n") != "ip netns add mk-0123456789ab\nip netns delete mk-0123456789ab" {
		t.Fatalf("namespace calls=%v", runner.calls)
	}
}

func TestNetnsTargetAcceptsOnlySupportedCanonicalForms(t *testing.T) {
	for input, expected := range map[string]string{
		"/proc/42/ns/net":        "42",
		"/run/netns/pod-one":     "pod-one",
		"/var/run/netns/pod.two": "pod.two",
	} {
		actual, err := netnsTarget(input)
		if err != nil || actual != expected {
			t.Fatalf("netnsTarget(%q)=%q,%v", input, actual, err)
		}
	}
	for _, input := range []string{"/tmp/netns", "/run/netns/../escape", "/proc/self/ns/net"} {
		if _, err := netnsTarget(input); err == nil {
			t.Fatalf("netnsTarget(%q) succeeded", input)
		}
	}
}

func TestOpenTUNRejectsUnsafeNamespaceBeforeSetns(t *testing.T) {
	if _, err := OpenTUN(Endpoint{NetNS: "/tmp/unsafe", IfName: "eth0"}); err == nil {
		t.Fatal("unsafe namespace accepted")
	}
}

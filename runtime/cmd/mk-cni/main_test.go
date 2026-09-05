package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
)

type fakeCaller struct {
	requests []network.Request
	response network.Response
	err      error
}

func (f *fakeCaller) Call(_ context.Context, request network.Request) (network.Response, error) {
	f.requests = append(f.requests, request)
	return f.response, f.err
}

func validConfig(t *testing.T) []byte {
	t.Helper()
	return []byte(fmt.Sprintf(`{"cniVersion":"1.0.0","name":"multikernel","type":"multikernel","socket":"/run/mknetd.sock","cacheDir":%q}`, t.TempDir()))
}

func TestAddReturnsAllocatedInterfaceIPAndDNS(t *testing.T) {
	fake := &fakeCaller{response: network.Response{Endpoint: &network.Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box",
		Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", DNS: network.DNS{Nameservers: []string{"1.1.1.1"}},
	}}}
	value, err := run(context.Background(), validConfig(t), environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, fake)
	if err != nil {
		t.Fatal(err)
	}
	result := value.(cniResult)
	if result.IPs[0].Address != "172.31.0.2/30" || result.Interfaces[0].Sandbox != "/run/netns/box" || result.DNS.Nameservers[0] != "1.1.1.1" {
		t.Fatalf("ADD result = %+v", result)
	}
	if len(fake.requests) != 1 || fake.requests[0].Method != "ADD" {
		t.Fatalf("requests = %+v", fake.requests)
	}
}

func TestCheckAndDeleteProduceNoResultAndDeleteAllowsEmptyNetNS(t *testing.T) {
	input := validConfig(t)
	configuration, err := validateConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	env := environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}
	if err = writeCache(configuration, env, cacheRecord{Version: 1, Generation: "0123456789abcdef0123456789abcdef", NetNS: env.NetNS}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeCaller{}
	for _, test := range []environment{
		{Command: "CHECK", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"},
		{Command: "DEL", ContainerID: "box", IfName: "eth0"},
	} {
		value, err := run(context.Background(), input, test, fake)
		if err != nil || value != nil {
			t.Fatalf("%s value=%v error=%v", test.Command, value, err)
		}
	}
	if len(fake.requests) != 2 || fake.requests[0].Endpoint.Generation == "" || fake.requests[1].Endpoint.NetNS != "/run/netns/box" {
		t.Fatalf("cached requests = %+v", fake.requests)
	}
}

func TestVersionDoesNotRequireConfigOrDaemon(t *testing.T) {
	value, err := run(context.Background(), nil, environment{Command: "VERSION"}, nil)
	if err != nil || len(value.(cniVersions).SupportedVersions) != 1 {
		t.Fatalf("VERSION value=%+v error=%v", value, err)
	}
}

func TestCNIRejectsUnknownFieldsUnsafeIdentityAndDaemonErrors(t *testing.T) {
	valid := validConfig(t)
	badConfig := append(valid[:len(valid)-1], []byte(`,"extra":true}`)...)
	if _, err := run(context.Background(), badConfig, environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, &fakeCaller{}); err == nil {
		t.Fatal("unknown config field accepted")
	}
	if _, err := run(context.Background(), valid, environment{Command: "ADD", ContainerID: "../box", IfName: "eth0", NetNS: "/run/netns/box"}, &fakeCaller{}); err == nil {
		t.Fatal("unsafe container ID accepted")
	}
	injected := &network.APIError{Code: "RESOURCE_EXHAUSTED", Message: "full"}
	if _, err := run(context.Background(), valid, environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, &fakeCaller{err: injected}); !errors.Is(err, injected) || errorCode(err) != 11 {
		t.Fatalf("daemon error=%v code=%d", err, errorCode(err))
	}
}

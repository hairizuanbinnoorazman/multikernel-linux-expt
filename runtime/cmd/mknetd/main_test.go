//go:build linux

package main

import (
	"reflect"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
)

func TestParseConfigurationIsExplicitAndDeterministic(t *testing.T) {
	value, err := parseConfiguration([]string{"-socket", "/run/test.sock", "-state-dir", "/var/lib/test", "-subnet", "10.40.0.0/24", "-egress", "ens4", "-mtu", "1300", "-allowed-uid", "42", "-dns", "10.0.0.2,10.0.0.3"})
	if err != nil {
		t.Fatal(err)
	}
	if value.socket != "/run/test.sock" || value.stateDir != "/var/lib/test" || value.egress != "ens4" || value.mtu != 1300 || value.allowedUID != 42 || !reflect.DeepEqual(value.dns, network.DNS{Nameservers: []string{"10.0.0.2", "10.0.0.3"}}) {
		t.Fatalf("configuration = %+v", value)
	}
}

func TestParseConfigurationRejectsImplicitOwnership(t *testing.T) {
	for _, arguments := range [][]string{
		{},
		{"-egress", "ens4", "-socket", "relative"},
		{"-egress", "ens4", "-state-dir", "relative"},
		{"-egress", "ens4", "-allowed-uid", "-1"},
		{"-egress", "ens4", "-dns", "10.0.0.2,"},
		{"-egress", "ens4", "extra"},
	} {
		if _, err := parseConfiguration(arguments); err == nil {
			t.Fatalf("arguments %v succeeded", arguments)
		}
	}
}

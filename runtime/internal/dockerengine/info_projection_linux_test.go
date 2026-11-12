//go:build linux

package dockerengine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func ambiguousInfoDocuments() map[string]string {
	return map[string]string{
		"duplicate-driver":           strings.Replace(validInfo, `"Driver":"vfs"`, `"Driver":"overlay2","Driver":"vfs"`, 1),
		"escaped-driver-duplicate":   strings.Replace(validInfo, `"Driver":"vfs"`, `"Dr\u0069ver":"overlay2","Driver":"vfs"`, 1),
		"case-collision-driver":      strings.Replace(validInfo, `"Driver":"vfs"`, `"Driver":"overlay2","driver":"vfs"`, 1),
		"case-alias-os":              strings.Replace(validInfo, `"OSType"`, `"ostype"`, 1),
		"case-alias-root":            strings.Replace(validInfo, `"DockerRootDir"`, `"dockerrootdir"`, 1),
		"case-alias-security":        strings.Replace(validInfo, `"SecurityOptions"`, `"securityoptions"`, 1),
		"unicode-fold-security":      strings.Replace(validInfo, `"SecurityOptions"`, `"\u017fecurityOptions"`, 1),
		"nested-duplicate-extension": strings.TrimSuffix(validInfo, "}") + `,"Extension":{"SYNTHETIC_PRIVATE_NAME":"first","SYNTHETIC_PRIVATE_NAME":"last"}}`,
		"duplicate-extension":        strings.TrimSuffix(validInfo, "}") + `,"SYNTHETIC_PRIVATE_NAME":1,"SYNTHETIC_PRIVATE_NAME":2}`,
		"oversize-extension":         strings.TrimSuffix(validInfo, "}") + `,"Extension":"` + strings.Repeat("x", 64<<10) + `"}`,
	}
}

func TestInfoProjectionRejectsAmbiguousDocuments(t *testing.T) {
	for name, document := range ambiguousInfoDocuments() {
		t.Run(name, func(t *testing.T) {
			issue := validateInfo([]byte(document))
			if issue == nil || issue.Error() != "Docker daemon does not match offline rootless profile" {
				t.Fatal("ambiguous info accepted or document reflected")
			}
		})
	}
}

func TestInfoProjectionKeepsExtensibleCanonicalResponses(t *testing.T) {
	for name, extension := range map[string]string{
		"canonical-only":        "",
		"docker-fields":         `,"ID":"synthetic-id","Name":"synthetic-name","NCPU":2,"Plugins":{"Network":["bridge"],"Volume":["local"]}`,
		"unrelated-case-fields": `,"extension":1,"Extension":2,"Labels":["SYNTHETIC_PRIVATE=value"],"Nested":{"driver":"unrelated"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if validateInfo([]byte(strings.TrimSuffix(validInfo, "}")+extension+"}")) != nil {
				t.Fatal("canonical extensible info refused")
			}
		})
	}
}

func TestReadinessRejectsAmbiguousInfoOverHTTP(t *testing.T) {
	for _, name := range []string{"canonical", "duplicate-driver", "case-collision-driver", "unicode-fold-security", "nested-duplicate-extension"} {
		t.Run(name, func(t *testing.T) {
			document := validInfo
			if name != "canonical" {
				document = ambiguousInfoDocuments()[name]
			}
			var mu sync.Mutex
			requests := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, r.URL.Path)
				mu.Unlock()
				if r.Method != http.MethodGet {
					http.Error(w, "fixture method refused", 400)
					return
				}
				if r.URL.Path == "/_ping" {
					io.WriteString(w, "OK")
					return
				}
				if r.URL.Path == "/info" {
					io.WriteString(w, document)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			client := server.Client()
			transport := client.Transport.(*http.Transport).Clone()
			transport.Proxy = nil
			// Retain real HTTP parsing and sockets, redirect only the fixed fixture
			// host through a trusted transport wrapper; no Docker peer is contacted.
			client.Transport = probeRoundTripper(func(r *http.Request) (*http.Response, error) {
				cloned := r.Clone(r.Context())
				cloned.URL.Scheme = "http"
				cloned.URL.Host = strings.TrimPrefix(server.URL, "http://")
				return transport.RoundTrip(cloned)
			})
			defer transport.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			issue := probeAPI(ctx, client)
			if name == "canonical" {
				if issue != nil {
					t.Fatal("canonical HTTP readiness refused")
				}
			} else if issue == nil || issue.Error() != "Docker daemon does not match offline rootless profile" {
				t.Fatal("ambiguous HTTP info accepted or reflected")
			}
			mu.Lock()
			observed := fmt.Sprint(requests)
			mu.Unlock()
			if observed != "[/_ping /info]" {
				t.Fatal("probe changed request count or paths")
			}
		})
	}
}

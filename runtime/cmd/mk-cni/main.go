package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

var cniIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type config struct {
	CNIVersion string `json:"cniVersion"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Socket     string `json:"socket"`
	CacheDir   string `json:"cacheDir"`
}

type environment struct {
	Command     string
	ContainerID string
	NetNS       string
	IfName      string
}

type caller interface {
	Call(context.Context, network.Request) (network.Response, error)
}

type cniInterface struct {
	Name    string `json:"name"`
	Sandbox string `json:"sandbox,omitempty"`
}
type cniIP struct {
	Version   string `json:"version"`
	Address   string `json:"address"`
	Gateway   string `json:"gateway"`
	Interface int    `json:"interface"`
}
type cniResult struct {
	CNIVersion string         `json:"cniVersion"`
	Interfaces []cniInterface `json:"interfaces"`
	IPs        []cniIP        `json:"ips"`
	DNS        network.DNS    `json:"dns"`
}
type cniVersions struct {
	CNIVersion        string   `json:"cniVersion"`
	SupportedVersions []string `json:"supportedVersions"`
}
type cniError struct {
	CNIVersion string `json:"cniVersion"`
	Code       int    `json:"code"`
	Message    string `json:"msg"`
	Details    string `json:"details"`
}

func requestID(command string) (string, error) {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return "cni-" + command + "-" + hex.EncodeToString(data), nil
}

func validateConfig(data []byte) (config, error) {
	var value config
	if err := protocol.StrictDecode(data, &value); err != nil {
		return value, err
	}
	if value.CNIVersion != "1.0.0" || !cniIdentifier.MatchString(value.Name) || value.Type != "multikernel" ||
		!filepath.IsAbs(value.Socket) || filepath.Clean(value.Socket) != value.Socket ||
		!filepath.IsAbs(value.CacheDir) || filepath.Clean(value.CacheDir) != value.CacheDir {
		return value, errors.New("CNI config requires cniVersion 1.0.0, a valid name, type multikernel, and absolute socket/cacheDir")
	}
	return value, nil
}

type cacheRecord struct {
	Version    int    `json:"version"`
	Generation string `json:"generation"`
	NetNS      string `json:"netns"`
}

func cachePath(configuration config, env environment) string {
	digest := sha256.Sum256([]byte(configuration.Name + "\x00" + env.ContainerID + "\x00" + env.IfName))
	return filepath.Join(configuration.CacheDir, hex.EncodeToString(digest[:])+".json")
}

func validateCacheDirectory(path string, create bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("CNI cache directory must be absolute and canonical")
	}
	if create {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || identity.Uid != uint32(os.Geteuid()) {
		return errors.New("CNI cache directory must be a private caller-owned real directory")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return errors.New("CNI cache directory may not contain symlinks")
	}
	return nil
}

func readCache(configuration config, env environment) (cacheRecord, error) {
	var record cacheRecord
	if err := validateCacheDirectory(configuration.CacheDir, false); err != nil {
		return record, err
	}
	path := cachePath(configuration, env)
	info, err := os.Lstat(path)
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > 4096 {
		return record, errors.New("CNI endpoint cache must be a private bounded regular file")
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return record, fmt.Errorf("open CNI endpoint cache: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return record, errors.New("CNI endpoint cache identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return record, errors.New("CNI endpoint cache changed or exceeded its read bound")
	}
	if err = protocol.StrictDecode(data, &record); err != nil || record.Version != 1 || !endpointGeneration.MatchString(record.Generation) || !filepath.IsAbs(record.NetNS) {
		return record, errors.New("CNI endpoint cache is malformed")
	}
	return record, nil
}

var endpointGeneration = regexp.MustCompile(`^[0-9a-f]{32}$`)

func writeCache(configuration config, env environment, record cacheRecord) (retErr error) {
	if err := validateCacheDirectory(configuration.CacheDir, true); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(configuration.CacheDir, ".endpoint.*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		if retErr != nil {
			_ = os.Remove(name)
		}
	}()
	if err = temporary.Chmod(0600); err == nil {
		_, err = temporary.Write(append(data, '\n'))
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, cachePath(configuration, env)); err != nil {
		return err
	}
	return syncDirectory(configuration.CacheDir)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func validateAllocatedEndpoint(requested, allocated *network.Endpoint) error {
	if allocated == nil || !endpointGeneration.MatchString(allocated.Generation) {
		return errors.New("mknetd returned an invalid endpoint generation")
	}
	if allocated.ContainerID != requested.ContainerID || allocated.NetworkName != requested.NetworkName ||
		allocated.IfName != requested.IfName || allocated.NetNS != requested.NetNS || allocated.Owner != "cni" ||
		allocated.ManagedNamespace || allocated.SandboxID != "" || allocated.SandboxGeneration != "" || allocated.State != "READY" {
		return errors.New("mknetd ADD response identity differs from the exact CNI request")
	}
	ip, subnet, err := net.ParseCIDR(allocated.Address)
	if err != nil || ip.To4() == nil {
		return errors.New("mknetd returned an invalid IPv4 address")
	}
	ones, bits := subnet.Mask.Size()
	gateway := net.ParseIP(allocated.Gateway)
	if bits != 32 || ones != 30 || gateway == nil || gateway.To4() == nil || !subnet.Contains(gateway) || gateway.Equal(ip) {
		return errors.New("mknetd returned an invalid IPv4 /30 gateway")
	}
	if allocated.MTU < 576 || allocated.MTU > 65515 {
		return errors.New("mknetd returned an invalid MTU")
	}
	if len(allocated.DNS.Nameservers) > 8 || len(allocated.DNS.Search) > 8 || len(allocated.DNS.Options) > 8 {
		return errors.New("mknetd returned an oversized DNS policy")
	}
	for _, server := range allocated.DNS.Nameservers {
		if net.ParseIP(server) == nil {
			return errors.New("mknetd returned an invalid DNS server")
		}
	}
	return nil
}

func rollbackAllocatedEndpoint(client caller, allocated *network.Endpoint) error {
	if allocated == nil || !endpointGeneration.MatchString(allocated.Generation) {
		return errors.New("cannot safely roll back an endpoint without its valid generation")
	}
	rollbackID, err := requestID("DEL")
	if err != nil {
		return err
	}
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = client.Call(rollbackCtx, network.Request{Version: 1, RequestID: rollbackID, Method: "DEL", Endpoint: allocated})
	return err
}

func run(ctx context.Context, input []byte, env environment, client caller) (any, error) {
	if env.Command == "VERSION" {
		return cniVersions{CNIVersion: "1.0.0", SupportedVersions: []string{"1.0.0"}}, nil
	}
	configuration, err := validateConfig(input)
	if err != nil {
		return nil, err
	}
	if !cniIdentifier.MatchString(env.ContainerID) || !cniIdentifier.MatchString(env.IfName) {
		return nil, errors.New("CNI_CONTAINERID and CNI_IFNAME must be valid identifiers")
	}
	if (env.Command == "ADD" || env.Command == "CHECK") && (env.NetNS == "" || env.NetNS[0] != '/') {
		return nil, errors.New("CNI_NETNS must be absolute for ADD and CHECK")
	}
	id, err := requestID(env.Command)
	if err != nil {
		return nil, err
	}
	endpoint := &network.Endpoint{ContainerID: env.ContainerID, NetworkName: configuration.Name, IfName: env.IfName, NetNS: env.NetNS}
	if env.Command == "CHECK" || env.Command == "DEL" {
		record, cacheErr := readCache(configuration, env)
		if cacheErr == nil {
			endpoint.Generation = record.Generation
			if endpoint.NetNS == "" {
				endpoint.NetNS = record.NetNS
			}
		} else if env.Command == "CHECK" || !errors.Is(cacheErr, os.ErrNotExist) {
			return nil, cacheErr
		}
	}
	response, err := client.Call(ctx, network.Request{Version: 1, RequestID: id, Method: env.Command, Endpoint: endpoint})
	if err != nil {
		return nil, err
	}
	if env.Command == "DEL" {
		if err = os.Remove(cachePath(configuration, env)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			if err = syncDirectory(configuration.CacheDir); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if env.Command == "CHECK" {
		return nil, nil
	}
	if env.Command != "ADD" || response.Endpoint == nil {
		return nil, errors.New("unsupported CNI_COMMAND")
	}
	allocated := response.Endpoint
	if err = validateAllocatedEndpoint(endpoint, allocated); err != nil {
		return nil, errors.Join(err, rollbackAllocatedEndpoint(client, allocated))
	}
	if err = writeCache(configuration, env, cacheRecord{Version: 1, Generation: allocated.Generation, NetNS: allocated.NetNS}); err != nil {
		return nil, errors.Join(fmt.Errorf("persist CNI generation cache: %w", err), rollbackAllocatedEndpoint(client, allocated))
	}
	return cniResult{
		CNIVersion: configuration.CNIVersion,
		Interfaces: []cniInterface{{Name: allocated.IfName, Sandbox: allocated.NetNS}},
		IPs:        []cniIP{{Version: "4", Address: allocated.Address, Gateway: allocated.Gateway, Interface: 0}},
		DNS:        allocated.DNS,
	}, nil
}

func errorCode(err error) int {
	var api *network.APIError
	if errors.As(err, &api) {
		switch api.Code {
		case "INVALID_ARGUMENT":
			return 4
		case "NOT_FOUND":
			return 7
		case "RESOURCE_EXHAUSTED":
			return 11
		}
	}
	return 100
}

func main() {
	if buildinfo.PrintRequested(os.Stdout, "mk-cni", os.Args[1:]) {
		return
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	env := environment{Command: os.Getenv("CNI_COMMAND"), ContainerID: os.Getenv("CNI_CONTAINERID"), NetNS: os.Getenv("CNI_NETNS"), IfName: os.Getenv("CNI_IFNAME")}
	var client caller
	if env.Command != "VERSION" {
		configuration, configErr := validateConfig(input)
		if configErr == nil {
			client = network.Client{Path: configuration.Socket, Timeout: 10 * time.Second}
		}
	}
	result, err := run(context.Background(), input, env, client)
	encoder := json.NewEncoder(os.Stdout)
	if err != nil {
		_ = encoder.Encode(cniError{CNIVersion: "1.0.0", Code: errorCode(err), Message: "multikernel CNI request failed", Details: err.Error()})
		os.Exit(1)
	}
	if result != nil {
		if err = encoder.Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

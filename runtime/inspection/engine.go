// Package inspection defines the read-only extension on the existing shim socket.
package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const Service = "multikernel.inspection.v1.Inspection"
const Method = "InspectDockerEngine"
const MaxRequest = 1024
const MaxReport = 2048

type Request struct {
	Version     int    `json:"version"`
	Namespace   string `json:"namespace"`
	ContainerID string `json:"container_id"`
	SandboxID   string `json:"sandbox_id"`
	Generation  string `json:"generation"`
}

type Report struct {
	Version     int                              `json:"version"`
	Owner       Request                          `json:"owner"`
	Observation protocol.DockerEngineObservation `json:"observation"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var sandbox = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var generation = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (r Request) Validate() error {
	if r.Version != 1 || !identifier.MatchString(r.Namespace) || !identifier.MatchString(r.ContainerID) || !sandbox.MatchString(r.SandboxID) || !generation.MatchString(r.Generation) {
		return errors.New("invalid engine inspection owner")
	}
	return nil
}
func exact(data []byte, max int, keys ...string) error {
	var fields map[string]json.RawMessage
	if len(data) > max || protocol.StrictDecode(data, &fields) != nil || len(fields) != len(keys) {
		return errors.New("invalid engine inspection fields")
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return errors.New("invalid engine inspection fields")
		}
	}
	return nil
}
func DecodeRequest(data []byte) (Request, error) {
	var r Request
	if exact(data, MaxRequest, "version", "namespace", "container_id", "sandbox_id", "generation") != nil || protocol.StrictDecode(data, &r) != nil || r.Validate() != nil {
		return Request{}, errors.New("invalid engine inspection request")
	}
	return r, nil
}
func DecodeReport(data []byte, owner Request) (Report, error) {
	var r Report
	if exact(data, MaxReport, "version", "owner", "observation") != nil || protocol.StrictDecode(data, &r) != nil || r.Version != 1 || r.Owner != owner || owner.Validate() != nil || r.Observation.Validate() != nil {
		return Report{}, errors.New("invalid engine inspection report")
	}
	// Validate the nested owner with the same exact key policy as requests.
	var fields map[string]json.RawMessage
	if protocol.StrictDecode(data, &fields) != nil {
		return Report{}, errors.New("invalid engine inspection report")
	}
	if _, err := DecodeRequest(fields["owner"]); err != nil {
		return Report{}, err
	}
	return r, nil
}

type Observer func(context.Context, Request) (Report, error)

// Register uses the existing shim server and its peer authentication. BytesValue
// contains bounded strict JSON; it never transports a token, command or path.
func Register(server *ttrpc.Server, observer Observer) {
	server.RegisterService(Service, &ttrpc.ServiceDesc{Methods: map[string]ttrpc.Method{
		Method: func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
			var wire wrapperspb.BytesValue
			if unmarshal(&wire) != nil {
				return nil, status.Error(codes.InvalidArgument, "invalid engine inspection request")
			}
			request, err := DecodeRequest(wire.Value)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, "invalid engine inspection request")
			}
			report, err := observer(ctx, request)
			if err != nil {
				code := status.Code(err)
				switch code {
				case codes.Canceled, codes.DeadlineExceeded, codes.InvalidArgument, codes.FailedPrecondition, codes.Unimplemented, codes.Unavailable:
				default:
					code = codes.Internal
				}
				return nil, status.Error(code, "Docker engine inspection unavailable")
			}
			data, err := json.Marshal(report)
			if err != nil {
				return nil, status.Error(codes.Internal, "engine inspection encoding failed")
			}
			if _, err = DecodeReport(data, request); err != nil {
				return nil, status.Error(codes.Internal, "invalid engine inspection result")
			}
			if ctx.Err() != nil {
				return nil, status.Error(codes.DeadlineExceeded, "Docker engine inspection unavailable")
			}
			return wrapperspb.Bytes(data), nil
		},
	}})
}

func Inspect(ctx context.Context, client *ttrpc.Client, owner Request) (Report, error) {
	if owner.Validate() != nil {
		return Report{}, errors.New("invalid engine inspection owner")
	}
	data, err := json.Marshal(owner)
	if err != nil {
		return Report{}, err
	}
	var wire wrapperspb.BytesValue
	if err = client.Call(ctx, Service, Method, wrapperspb.Bytes(data), &wire); err != nil {
		return Report{}, err
	}
	if ctx.Err() != nil {
		return Report{}, ctx.Err()
	}
	return DecodeReport(wire.Value, owner)
}

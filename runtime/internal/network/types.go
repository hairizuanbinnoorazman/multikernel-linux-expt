package network

import "time"

const ProtocolVersion = 1

type DNS struct {
	Nameservers []string `json:"nameservers,omitempty"`
	Domain      string   `json:"domain,omitempty"`
	Search      []string `json:"search,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Endpoint struct {
	ContainerID       string    `json:"container_id"`
	NetworkName       string    `json:"network_name"`
	IfName            string    `json:"if_name"`
	NetNS             string    `json:"netns"`
	Owner             string    `json:"owner"`
	ManagedNamespace  bool      `json:"managed_namespace,omitempty"`
	SandboxID         string    `json:"sandbox_id,omitempty"`
	SandboxGeneration string    `json:"sandbox_generation,omitempty"`
	Generation        string    `json:"generation"`
	Address           string    `json:"address"`
	Gateway           string    `json:"gateway"`
	MTU               int       `json:"mtu"`
	DNS               DNS       `json:"dns"`
	State             string    `json:"state"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	RXPackets         uint64    `json:"rx_packets"`
	TXPackets         uint64    `json:"tx_packets"`
	RXDrops           uint64    `json:"rx_drops"`
	TXDrops           uint64    `json:"tx_drops"`
	Errors            uint64    `json:"errors"`
}

type Request struct {
	Version    int       `json:"version"`
	RequestID  string    `json:"request_id"`
	Method     string    `json:"method"`
	Endpoint   *Endpoint `json:"endpoint,omitempty"`
	Generation string    `json:"generation,omitempty"`
}

type Response struct {
	Version   int       `json:"version"`
	RequestID string    `json:"request_id"`
	Endpoint  *Endpoint `json:"endpoint,omitempty"`
	Error     *APIError `json:"error,omitempty"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

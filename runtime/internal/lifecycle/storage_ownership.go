package lifecycle

import "github.com/hairizuan/multikernel-linux-expt/runtime/protocol"

func (s *Service) StorageOwnershipAvailable() bool { return s.storage != nil }
func (s *Service) StorageOwnership() (protocol.StorageOwnershipReport, *protocol.Error) {
	if s.storage == nil {
		return protocol.StorageOwnershipReport{}, &protocol.Error{Code: "UNSUPPORTED", Message: "storage ownership is unavailable"}
	}
	return s.storage.Ownership()
}

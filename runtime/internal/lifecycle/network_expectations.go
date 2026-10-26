package lifecycle

import "github.com/hairizuan/multikernel-linux-expt/runtime/protocol"

func (s *Service) NetworkExpectations() (protocol.NetworkExpectationsReport, *protocol.Error) {
	return s.store.NetworkExpectationsSnapshot()
}

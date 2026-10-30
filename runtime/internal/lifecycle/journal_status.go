package lifecycle

import (
	"context"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func (s *Service) JournalStatusAvailable() bool { return s.store != nil }
func (s *Service) JournalStatus(ctx context.Context) (protocol.JournalStatusReport, *protocol.Error) {
	if s.store == nil {
		return protocol.JournalStatusReport{}, &protocol.Error{Code: "UNSUPPORTED", Message: "journal status unavailable"}
	}
	return s.store.JournalStatus(ctx)
}

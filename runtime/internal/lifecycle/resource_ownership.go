package lifecycle

import (
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
)

// ResourceOwnership brackets a storage snapshot with lifecycle projections.
// It observes changes, never takes mutation locks or consults a backend.
func (s *Service) ResourceOwnership() (protocol.ResourceOwnershipReport, *protocol.Error) {
	if s.storage == nil {
		return protocol.ResourceOwnershipReport{}, &protocol.Error{Code: "UNSUPPORTED", Message: "resource ownership is unavailable"}
	}
	before, failure := s.store.ResourceOwnershipSnapshot()
	if failure != nil {
		return protocol.ResourceOwnershipReport{}, failure
	}
	storage, descriptors, failure := s.storage.OwnershipWithDescriptors()
	if failure != nil {
		return protocol.ResourceOwnershipReport{}, failure
	}
	after, failure := s.store.ResourceOwnershipSnapshot()
	if failure != nil {
		return protocol.ResourceOwnershipReport{}, failure
	}
	if !reflect.DeepEqual(before, after) {
		return protocol.ResourceOwnershipReport{}, &protocol.Error{Code: "UNAVAILABLE", Message: "resource ownership changed during observation", Retryable: true}
	}
	after.Storage = storage
	after.StorageDescriptors = descriptors
	if after.Validate() != nil {
		return protocol.ResourceOwnershipReport{}, &protocol.Error{Code: "OPERATOR_ACTION", Message: "resource ownership metadata is invalid"}
	}
	return after, nil
}

//go:build linux

package main

import (
	"encoding/json"
	"errors"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
)

const createIntentName = ".multikernel-create.json"

// This marker records uncertainty, not cleanup authority. Never put credentials,
// configuration, guest output, process IDs or destructive commands in it.
func (s *service) refusePendingCreate() error {
	if err := s.ensureBundleIdentity(); err != nil {
		return errors.New("bundle directory identity changed or unavailable for create intent")
	}
	present, err := s.bundleDirectory.EntryOrRemovalPending(createIntentName, 256)
	if err != nil || present {
		return errors.New("pending create intent requires inspection")
	}
	return nil
}

func (s *service) beginCreateIntent() error {
	if err := s.refusePendingCreate(); err != nil {
		return err
	}
	if !runtimeIdentifier.MatchString(s.id) || !runtimeIdentifier.MatchString(s.namespace) {
		return errors.New("create intent identity is invalid")
	}
	data, err := json.Marshal(struct {
		Version   int                         `json:"version"`
		Namespace string                      `json:"namespace"`
		Task      string                      `json:"task"`
		Bundle    rootfspkg.DirectoryIdentity `json:"bundle_identity"`
	}{1, s.namespace, s.id, s.bundleIdentity})
	if err != nil || len(data) > 1024 {
		return errors.New("create intent encoding failed")
	}
	created, identity, err := s.bundleDirectory.PublishExclusiveRegularIdentity(createIntentName, data, 0600)
	if created {
		s.createIntentIdentity = &identity
	}
	if err != nil || !created {
		// Publication can fail after creating the name. Keep any captured handle
		// and fail closed; never infer permission to remove a foreign marker.
		s.createCleanupPending = true
		return errors.New("create intent publication failed")
	}
	return nil
}

func (s *service) clearCreateIntent() error {
	if s.createIntentIdentity == nil {
		return nil
	}
	if err := s.ensureBundleIdentity(); err != nil {
		return errors.New("create intent bundle identity changed")
	}
	removed, err := s.bundleDirectory.RemoveIfIdentity(createIntentName, *s.createIntentIdentity)
	if err != nil || !removed {
		return errors.New("create intent removal requires inspection")
	}
	s.createIntentIdentity = nil
	return nil
}

package rootfs

import (
	"errors"
	"fmt"
	"strings"
)

var errMountBudget = errors.New("rootfs mount budget rejected")

func mountBudgetError(reason string) error { return fmt.Errorf("%w: %s", errMountBudget, reason) }

const (
	maximumRootfsMounts        = 8
	maximumRootfsOverlayLayers = 128 // Includes a caller upper projected as lower.
	maximumRootfsSourceViews   = 256
)

// Validate the whole request before opening any filesystem object. Each layer
// consumes an original descriptor and a detached mount view during preparation.
func validateMountBudget(input []Mount) error {
	if len(input) > maximumRootfsMounts {
		return mountBudgetError("at most eight rootfs mounts are supported")
	}
	views := 0
	for _, item := range input {
		if len(item.Options) > 64 {
			return mountBudgetError("too many rootfs mount options")
		}
		for _, option := range item.Options {
			if len(option) > 4096 {
				return mountBudgetError("rootfs mount option exceeds limit")
			}
		}
		if item.Type == "bind" || item.Type == "none" {
			views++
		}
		if item.Type != "overlay" {
			continue
		}
		seen := map[string]bool{}
		layers := 0
		for _, option := range item.Options {
			for _, prefix := range []string{"lowerdir=", "upperdir=", "workdir="} {
				if !strings.HasPrefix(option, prefix) {
					continue
				}
				if seen[prefix] {
					return mountBudgetError("duplicate overlay path option")
				}
				seen[prefix] = true
				if prefix == "lowerdir=" {
					layers += strings.Count(strings.TrimPrefix(option, prefix), ":") + 1
				}
				if prefix == "upperdir=" {
					layers++
				}
			}
		}
		if layers > maximumRootfsOverlayLayers {
			return mountBudgetError("overlay exceeds 128 source layers")
		}
		views += layers
	}
	if views > maximumRootfsSourceViews {
		return mountBudgetError("rootfs request exceeds 256 source views")
	}
	return nil
}

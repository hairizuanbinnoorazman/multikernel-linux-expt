package doctor

// HealthObservationStatus checks the typed v8 report contract used by internal
// diagnostic publishers and later aggregate observers. It never upgrades a
// component observation into readiness or admission authority.
func HealthObservationStatus(r HealthReport) (string, bool) {
	if r.Version != 8 || r.Scope != "operational-journal-and-recorded-network-health-observations" || r.AdmissionGuaranteed {
		return "", false
	}
	if r.Operational == nil && r.Journal == nil && r.Network == nil {
		return r.Status, r.Status == "inconclusive" && !r.BootStable
	}
	if r.Operational == nil || r.Journal == nil || r.Network == nil || !r.BootStable || r.Journal.Validate() != nil || r.Network.Validate() != nil {
		return "", false
	}
	op := r.Operational
	if op.Version != 7 || op.Scope != "resource-network-pool-and-compiled-component-observations" || !op.BootStable || op.BootID != r.BootID || r.Network.PrimaryBootID != r.BootID || op.AdmissionGuaranteed || op.ArtifactVerified || op.KernelVerified {
		return "", false
	}
	status := op.Status
	if status != "consistent" && status != "degraded" && status != "inconclusive" {
		return "", false
	}
	if r.Journal.Status == "failed" || r.Network.Status == "degraded" {
		status = "degraded"
	} else if r.Journal.Status == "pending" && status != "degraded" {
		status = "inconclusive"
	}
	return r.Status, r.Status == status
}

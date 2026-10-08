#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

source_root=${1:?usage: test-runtime-task-v2-exhaustive-vm.sh SOURCE_ROOT}
iterations=${MK_RACE_ITERATIONS:-20}
scratch=$(mktemp -d -p /var/tmp tv2.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
mkdir -p "$scratch/c"
chmod 0700 "$scratch" "$scratch/c"
export GOCACHE="$scratch/c" GOTMPDIR="$scratch" TMPDIR="$scratch"
package=./cmd/containerd-shim-multikernel-v2
groups=0
total=0

[[ $iterations =~ ^[1-9][0-9]*$ ]]
for path in "$source_root/runtime/go.mod" "$source_root/scripts/audit-runtime-final-resources-live.sh"; do
	test -f "$path"
done

observe() { local key=$1; shift; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"; }

run_group() {
	local name=$1
	shift
	local tests=("$@") regex listed output test_name
	regex=$(IFS='|'; printf '^(%s)$' "${tests[*]}")
	listed=$(go test "$package" -list "$regex" | awk '/^Test/ {print}')
	for test_name in "${tests[@]}"; do grep -Fxq "$test_name" <<<"$listed"; done
	[[ $(grep -c '^Test' <<<"$listed") -eq ${#tests[@]} ]]
	observe "$name-selected-tests" "package=$package iterations=$iterations count=${#tests[@]}
$listed"
	output="$scratch/$name.log"
	go test -race -count="$iterations" -run "$regex" -v "$package" 2>&1 | tee "$output"
	! grep -Fq -- '--- SKIP:' "$output"
	! grep -Fq 'WARNING: DATA RACE' "$output"
	grep -Fq 'PASS' "$output"
	groups=$((groups + 1))
	total=$((total + ${#tests[@]}))
	observe "$name-result" "selected=${#tests[@]} iterations=$iterations skips=0 races=0 status=pass"
}

observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
kernel=$(uname -r)
go=$(go version)
source_root=$source_root
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')
shim_test_sha256=$(sha256sum "$source_root/runtime/cmd/containerd-shim-multikernel-v2/main_test.go" | awk '{print $1}')"
"$source_root/scripts/audit-runtime-final-resources-live.sh"

cd "$source_root/runtime"
run_group lifecycle \
	TestCancelCreateUsesExactOriginalIdentity \
	TestCreateAmbiguityCancellationRemovesPreparedArtifacts \
	TestCreateRejectsOCIValidationBeforeAllocationOrArtifacts \
	TestCreatePostValidationFailureRollbackMatrix \
	TestExecRollsBackProcessOnAgentFailure \
	TestExecCreationIntentReconcilesAmbiguousGuestMutation \
	TestRecoveredCreatedExecRequiresExactGuestOwnership \
	TestExecRollbackRetainsOwnershipUntilGuestDeletion \
	TestStartReturnsKillFailureAndRetainsMonitorAfterPersistenceFailure \
	TestStartRejectsGuestPIDOutsideTaskRangeAndRetainsOwnership \
	TestStartRejectsMismatchedGuestProcessStateAndRetainsOwnership \
	TestStartReconcilesAmbiguousGuestResultWithoutLosingOwnership \
	TestEnsureGuestProcessCreatedReconcilesBeforeMutation \
	TestEnsureGuestProcessCreatedReconcilesLostReply \
	TestStartedProcessCleanupTreatsAuthenticatedAbsenceAsSuccess \
	TestExecRejectsGuestUnrepresentableIDBeforeAgentContact \
	TestTaskStateGuardsRejectInvalidTransitionsBeforeAgentContact \
	TestPreCancelledTaskMutationsPreserveStateAndAvoidGuestContact \
	TestPreCancelledTaskReadsAndWaitAvoidGuestContact \
	TestTaskRPCsRejectNilAndMismatchedTaskIdentityBeforeMutation

run_group process_io \
	TestResizePtyStoresInitialSizeBeforeStart \
	TestOutputFIFOCanBeReattached \
	TestOutputFIFOKeeperBridgesWorkerDeath \
	TestInvalidProcessIOFailsBeforeCreateOrExecMutation \
	TestProcessStdinMustBeFIFO \
	TestStdinFIFOAcceptsLateAndRepeatedWriters \
	TestCloseIOStopsContinuouslyReadableStdinAfterInflightChunk \
	TestPendingStdinReplaysLostAcknowledgementExactlyOnce \
	TestStdinIntentPersistenceFailurePrecedesGuestMutation \
	TestStdinAcknowledgementPersistenceFailureRetainsReplayIdentity \
	TestOutputOffsetsAdvanceOnlyAfterDeliveryOrBoundedDrop \
	TestProcessOutputAndWaitRecoverTransportWithoutChangingOffsets \
	TestWaitProcessRejectsMismatchedStoppedStateUntilExactCompletion \
	TestProcessOutputReconnectHasOneOverallDeadline \
	TestProcessOutputDoesNotReplayAuthenticatedRemoteRejection \
	TestProcessOutputRequiresBoundedContiguousOffsetsAndKnownState \
	TestOutputOffsetRetriesWhenDurableAcknowledgementFails \
	TestWaitProcessDoesNotFabricateExitAfterReconnectBudget \
	TestWaitProcessDoesNotCompleteBeforeExitStateIsDurable \
	TestExitEventFlagRollsBackWhenRecoveryAcknowledgementFails \
	TestCloseIORetriesUntilGuestAcknowledges \
	TestCloseIOTerminalAndNonTerminalProcesses \
	TestProcessIOTeardownWhileFIFOPeersRemainAttached \
	TestCloseIOReconnectsTransportWithinCallerDeadline \
	TestCloseIOPersistsCreatedIntentAndDoesNotMutateStoppedProcess \
	TestStdinPumpRetriesRequestedCloseAndRecordsAcknowledgement \
	TestRecoveredNoFIFOPumpRetriesPendingClose \
	TestResizePtyRejectsInvalidRequests \
	TestResizePtyRollsBackIntentWhenGuestRejects \
	TestResizePtyRetriesLostReply \
	TestGuestProcessDeleteReconnectsAndConfirmsLostReply \
	TestKillForwardsOnlyForLiveKnownProcess \
	TestKillPersistsAndDeduplicatesLostSignalReply

run_group control_read \
	TestTaskEntryLockWaitHonorsCancellation \
	TestWaitPostExitLockHonorsCancellation \
	TestShutdownWaitsForEmptyOwnershipAndSealsService \
	TestShutdownClosesHeldBundleIdentity \
	TestShutdownRetainsOwnershipUntilDurableEventsFlush \
	TestInitTaskAPIsExposeHostNamespaceHolderAndRetainGuestIdentity \
	TestPauseResumeSignalsGuestAndPublishesTransitions \
	TestPauseRollsBackAlreadySignaledProcessOnPartialFailure \
	TestPauseResumeReturnRecoveryRollbackFailure \
	TestPauseResumeRepublishPriorStateAfterTransitionPersistenceFailure \
	TestStatsReturnsGuestProcessGroupMetrics \
	TestStatsRejectsAggregateOverflow \
	TestStatsRetriesLostGuestReply \
	TestUpdateAndCheckpointAreExcludedWithoutMutation

run_group delete_event_recovery \
	TestEventJournalPreservesOrderAndReplaysAfterFailure \
	TestEventJournalCompleteLifecycleOrderAndPersistenceFailureMatrix \
	TestEventJournalRetriesWithoutAnotherLifecycleRequest \
	TestEventJournalPersistenceFailurePrecedesPublication \
	TestEventJournalAckFailureRetainsPublishedEventForReplay \
	TestDeleteRepairsMissingExitEventBeforeDeleteEvent \
	TestDeleteTreatsAuthenticatedGuestAbsenceAsIdempotentSuccess \
	TestInitDeleteContinuesHostCleanupAfterGuestNetworkFailure \
	TestInitDeleteAcceptsAlreadyReapedRelayAndAbsentSandboxOnRetry \
	TestGuestShutdownRetriesQuiesceAndAcceptsLostTerminalReply \
	TestGuestNetworkCloseRetriesLostReply \
	TestGuestNetworkConfigurationRetriesLostReply \
	TestGuestShutdownRejectsUnprovenOrAuthenticatedFailure \
	TestDeleteRetainsRetryOwnershipAcrossGuestAndEventFailures \
	TestRecoveryStatePersistsGenerationProcessesAndOffsetsAtomically \
	TestRecoverExistingReconstructsExactSandboxProcessAndNetworkGeneration \
	TestFallbackCleanupPropagatesStopFailureBeforeDelete \
	TestFallbackCleanupPropagatesRootfsFailureAfterSandboxDelete \
	TestFallbackCleanupRequiresHeldAndDaemonBundleIdentity \
	TestFallbackCleanupRemovesAuthenticatedSocketWithoutRecovery

"$source_root/scripts/audit-runtime-final-resources-live.sh"
observe aggregate "methods=17 groups=$groups selected_tests=$total iterations=$iterations skips=0 races=0 status=pass"
echo G6_TASK_V2_EXHAUSTIVE_MATRIX_PASS

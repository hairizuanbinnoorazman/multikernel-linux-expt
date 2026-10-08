#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

source_root=${1:?usage: test-runtime-hostile-paths-vm.sh SOURCE_ROOT}
iterations=${MK_RACE_ITERATIONS:-100}
scratch=$(mktemp -d -p /var/tmp h.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
mkdir -p "$scratch/c"
chmod 0700 "$scratch" "$scratch/c"
export GOCACHE="$scratch/c" GOTMPDIR="$scratch" TMPDIR="$scratch"

[[ $iterations =~ ^[1-9][0-9]*$ ]]
for path in "$source_root/runtime/go.mod" "$source_root/scripts/audit-runtime-final-resources-live.sh"; do
	test -f "$path"
done

observe() { local key=$1; shift; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"; }

run_group() {
	local name=$1 package=$2
	shift 2
	local tests=("$@") regex listed output test_name
	regex=$(IFS='|'; printf '^(%s)$' "${tests[*]}")
	listed=$(go test "$package" -list "$regex" | awk '/^Test/ {print}')
	for test_name in "${tests[@]}"; do
		grep -Fxq "$test_name" <<<"$listed"
	done
	[[ $(grep -c '^Test' <<<"$listed") -eq ${#tests[@]} ]]
	observe "$name-selected-tests" "package=$package iterations=$iterations count=${#tests[@]}
$listed"
	output="$scratch/$name.log"
	go test -race -count="$iterations" -run "$regex" -v "$package" 2>&1 | tee "$output"
	! grep -Fq -- '--- SKIP:' "$output"
	grep -Fq 'PASS' "$output"
	observe "$name-result" "selected=${#tests[@]} iterations=$iterations skips=0 status=pass"
}

observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
kernel=$(uname -r)
go=$(go version)
source_root=$source_root
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')
go_mod_sha256=$(sha256sum "$source_root/runtime/go.mod" | awk '{print $1}')"
"$source_root/scripts/audit-runtime-final-resources-live.sh"

cd "$source_root/runtime"
run_group shim ./cmd/containerd-shim-multikernel-v2 \
	TestValidateServiceIdentityRejectsUnsafeValues \
	TestSandboxIDBindsCompleteNamespaceAndTaskWithoutAliases \
	TestCreateRejectsOCIValidationBeforeAllocationOrArtifacts \
	TestRootfsMountsAreSanitizedForDaemonAndValidated \
	TestProcessIOPathsRejectUnsafeIdentityAndSymlinkAncestors \
	TestProcessIOOpenRejectsReplacementAndCancellation \
	TestBoundProcessIORejectsReplacementBeforeStartOrRecovery \
	TestLoadOrCreateTokenReusesExactSafeIdentity \
	TestLoadOrCreateTokenFromHeldDirectory \
	TestLoadOrCreateTokenRejectsMalformedAndSymlinkState \
	TestRecoveryStateRejectsRuntimeDirectoryReplacement \
	TestRuntimeDirectoryHandoffRejectsPreparedReplacement \
	TestBundleNetworkNamespaceRequiresCanonicalOCIPath \
	TestServiceBundleIdentityPinsConfigAcrossPublicReplacement \
	TestInitialSupervisorCommandUsesHeldBundleAfterPublicReplacement \
	TestSupervisorWorkerUsesHeldBundleAfterPublicReplacement \
	TestEventJournalRefusesReplacedPathForWriteAndAcknowledgement \
	TestEventJournalRejectsPublicBundleReplacement \
	TestStaleRelayCleanupRejectsNonSocketPathsWithoutRemoval \
	TestStaleRelayCleanupRemovesExactSafeSocket \
	TestRecoveryStateIsBoundedStrictAndIdentityBound \
	TestFallbackCleanupRequiresHeldAndDaemonBundleIdentity

run_group rootfs ./internal/rootfs \
	TestValidateMountsRejectsHostileInputBeforeBackendUse \
	TestValidationRejectsSymlinkBundleAndPropagation \
	TestPrepareRejectsShimBundleIdentityReplacementBeforeMutation \
	TestMountPinsFilesystemInputsAcrossPathReplacement \
	TestMountRejectsSymlinkSubstitutionAtDescriptorOpen \
	TestMountRejectsReplacedTargetIdentityBeforeMount \
	TestBuildUsesPinnedArtifactDirectoriesAndRejectsNameReplacement \
	TestPreparedVerificationPinsArtifactsAndRejectsNameReplacement \
	TestCleanupRejectsWholeRootReplacementBeforeBackendMutation \
	TestCleanupRejectsArtifactDirectoryReplacementBeforeBackendMutation \
	TestRejectedMountTargetDoesNotUnmountReplacement \
	TestDescriptorAnchoredCleanupDoesNotFollowReplacementRoot \
	TestDescriptorAnchoredCleanupRejectsReplacedArtifact \
	TestCleanupQuarantineRejectsRemovalTimeReplacement \
	TestRootfsStoreRejectsDirectoryReplacementBeforePersistence \
	TestRootfsStoreRejectsSymlinkHardlinkAndPermissiveState \
	TestRootfsReconcileRefusesPostInitializationCleanupPathCorruption

run_group safefile ./internal/safefile \
	TestOpenDirectoryCreatesPrivatelyWithoutFollowingSymlinks \
	TestIdentityConditionedRemovalPreservesRacedReplacementAndRecoversQuarantine \
	TestCaptureRegularIdentityAllowsPublicModeButRejectsReplacement \
	TestPublicRegularAndSymlinkPublicationRetainExactIdentity \
	TestPrivateReadAndReplaceAreDescriptorAnchored \
	TestReplaceIdentityPublishesExactSuccessor \
	TestReplaceIdentityPreservesRacedSubstituteAndOriginal

run_group unixsocket ./internal/unixsocket \
	TestListenerRejectsUnsafeStalePaths \
	TestListenerReplacesOnlySafeStaleSocket \
	TestCapturedPathClosePreservesSocket \
	TestListenerCleanupPreservesReplacement \
	TestCapturedPathRemovesOnlyCapturedSocket \
	TestSocketRemovalQuarantineRestoresRemovalTimeReplacement \
	TestEnsureParentRejectsRootAsSocketPath

run_group storage ./internal/storage \
	TestReconcileRejectsStorageImageIdentityReplacement \
	TestStoreRejectsDirectoryReplacementBeforePersistence \
	TestStoreRejectsSymlinkAndUnknownOrDuplicateState \
	TestLinuxBackendRejectsImageParentReplacementAfterOpen \
	TestLinuxBackendRejectsServerExecutableReplacementAfterOpen \
	TestBackendLeaseValidationPrecedesPathDerivation

"$source_root/scripts/audit-runtime-final-resources-live.sh"
observe aggregate "groups=5 selected_tests=59 iterations=$iterations skips=0 status=pass"
echo G6_HOSTILE_PATH_RACE_MATRIX_PASS

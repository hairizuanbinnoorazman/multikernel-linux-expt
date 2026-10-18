# Repository artifacts and Git LFS

Run `make repo-setup` once after cloning. This installs repository-local Git LFS
filters and enables the tracked hooks in `.githooks/`. Git does not automatically
enable hooks from a cloned repository; this setup step is required for automatic
size-based tracking outside the evidence directory.

Every nonempty file under `evidence/` is tracked by Git LFS through
`.gitattributes`. Ordinary `git add` applies this rule automatically. Empty files
remain ordinary empty blobs, as required by Git LFS. Install Git LFS before
cloning, or run `git lfs pull` to materialize existing pointer files afterward.

The pre-commit hook also finds other staged files larger than1MiB, adds literal
filename rules to `.gitattributes` and converts their staged bytes into LFS
pointers. It preserves unstaged worktree edits and executable modes. It refuses
to absorb unstaged `.gitattributes` edits or replace an explicit custom filter;
resolve those attribute choices and retry the commit. It needs no network upload
while committing. The regular Git LFS pre-push hook uploads referenced payloads.

`make lfs-check` validates the complete index without changing it. The GitHub
policy workflow runs that check with LFS downloads disabled, so it checks Git
objects rather than downloading the full evidence archive. Hook behavior is
covered by `python3 scripts/test-lfs-policy.py` in isolated temporary repositories.

The2026-10-18 migration rewrites historical evidence blobs into LFS pointers.
Proof bytes, source history, executable bits, authors, commit dates and messages
are retained; commit IDs change. The complete original repository and unfinished
G7 work were backed up outside this repository before migration. The cleanup
report and old/new commit map are retained under
[evidence/repository-maintenance/history-lfs-20261018](../../evidence/repository-maintenance/history-lfs-20261018/README.md).

Existing clones must account for the rewritten history. Preserve local commits
and uncommitted files before updating them; use the migration map when locating
old commits cited in evidence. Fresh clones get the rewritten history. Do not
copy an old branch back onto the published main branch.

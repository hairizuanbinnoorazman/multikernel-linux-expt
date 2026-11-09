# Host control state is not an OCI source root

The OCI source resolver retains its canonical-path, traversal, symlink and
absolute-allowlist checks. An absolute allowlist does not permit exporting the
active host bundle or an observed filesystem ancestor containing it. The active
bundle's private `.multikernel` directory and its observed descendants are also
refused. Ordinary relative image roots and admitted absolute image roots remain
supported; `.multikernel-image` is not the reserved `.multikernel` component.

Overlap checks compare held device/inode identities, including an inherited
bundle descriptor after its public name is replaced. They do not rely only on
public path prefixes. Parent traversal is bounded to 256 directories, never
reads private file contents, and closes held descriptors on acceptance/refusal.
The source's opened identity must match the validated identity. The existing
builder descriptor-acquisition handshake still checks the source again.

Preparation checks this boundary after basic OCI validation and before bootstrap
lookup or payload copying. Validation-only callers still perform basic OCI
validation because containerd may not yet have mounted the image root. Scratch
and rootfs ownership journals can already exist at preparation time; rejection
does not claim that no bookkeeping was allocated. No rejected source is copied
into the child payload, and existing guarded rollback governs partial artifacts.

These checks observe directory ancestry in the current mount namespace. They
do not replace trusted mount-source policy, scan every bind-mount alias or other
bundle in the storage tree, or establish hostile child-kernel containment.
Keep absolute source allowlists narrow. Direct child kernels use no nested
virtualization and share primary trust after child-kernel compromise. Full
G7/G8, broader source authority review and actual child-Docker fault/recovery
qualification remain separate requirements.

The [source-bound baseline and GCE proof](../../../evidence/runtime-20261109/g7-g8-root-source-boundary/README.md)
qualify thirteen controls for private source rejection, accepted image roots,
held-descriptor rename, descriptor cleanup and actual pre-bootstrap builder
refusal. Rootfs race and required checks also pass. The fixture does not build
or boot a child image, and does not induce every mount-alias configuration.

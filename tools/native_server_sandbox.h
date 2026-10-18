#ifndef MULTIKERNEL_NATIVE_SERVER_SANDBOX_H
#define MULTIKERNEL_NATIVE_SERVER_SANDBOX_H

#include <errno.h>
#include <limits.h>
#include <stddef.h>
#include <stdint.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <unistd.h>

/* Linux seccomp/cBPF UAPI, kept independent of libc-specific Linux headers
 * because production helpers are built with musl. Unsupported ABIs fail at
 * compilation rather than installing a filter with the wrong syscall table. */
#if defined(__x86_64__) && !defined(__ILP32__)
#define NATIVE_AUDIT_ARCH 0xc000003eU
#elif defined(__aarch64__) && !defined(__AARCH64EB__)
#define NATIVE_AUDIT_ARCH 0xc00000b7U
#else
#error "storage server sandbox requires audited x86-64 or aarch64 Linux ABI"
#endif
struct native_filter { uint16_t code; uint8_t jt, jf; uint32_t k; };
struct native_filter_program { uint16_t len; struct native_filter *filter; };
#define NATIVE_LOAD(offset) {0x20, 0, 0, (offset)}
#define NATIVE_EQUAL(value, yes, no) {0x15, (yes), (no), (value)}
#define NATIVE_RETURN(value) {0x06, 0, 0, (value)}
#define NATIVE_ALLOW 0x7fff0000U
#define NATIVE_DENY (0x00050000U | EPERM)
#define NATIVE_KILL 0x80000000U
#define NATIVE_ALLOW_CALL(name) \
	NATIVE_EQUAL(SYS_##name, 0, 1), NATIVE_RETURN(NATIVE_ALLOW)
/* seccomp_data: nr at0, arch at4, args at16, each64 bits. Require both
 * halves of descriptor arguments to avoid accepting a truncated forged value. */
#define NATIVE_FD_CALL(name, descriptor) \
	NATIVE_EQUAL(SYS_##name, 0, 6), \
	NATIVE_LOAD(20), NATIVE_EQUAL(0, 0, 3), \
	NATIVE_LOAD(16), NATIVE_EQUAL((descriptor), 0, 1), \
	NATIVE_RETURN(NATIVE_ALLOW), NATIVE_RETURN(NATIVE_DENY)

/* Keep only image fd3, listener and diagnostic fds1/2. The held executable,
 * original/duplicate images, stdin and all accidental inherited descriptors
 * must be closed before the request sandbox. close_range needs Linux>=5.9;
 * absence or any failure aborts startup. No fallback scans a racy procfs list. */
static int native_server_close_descriptors(int image, int listener)
{
	if (image < 0 || listener < 4 || image == listener) {
		errno = EINVAL;
		return -1;
	}
	if (image != 3 && dup2(image, 3) < 0)
		return -1;
	if (close(0) < 0 && errno != EBADF)
		return -1;
	if (listener > 4 && syscall(SYS_close_range, 4U, (unsigned int)listener - 1U, 0U) < 0)
		return -1;
	return syscall(SYS_close_range, (unsigned int)listener + 1U, UINT_MAX, 0U);
}

static int native_server_install_filter(int listener)
{
	struct native_filter filter[] = {
		NATIVE_LOAD(4), NATIVE_EQUAL(NATIVE_AUDIT_ARCH, 1, 0), NATIVE_RETURN(NATIVE_KILL),
		NATIVE_LOAD(0),
		NATIVE_FD_CALL(pread64, 3), NATIVE_FD_CALL(pwrite64, 3),
		NATIVE_FD_CALL(fdatasync, 3), NATIVE_FD_CALL(fsync, 3),
#ifdef SYS_accept
		NATIVE_FD_CALL(accept, (uint32_t)listener),
#endif
		NATIVE_FD_CALL(accept4, (uint32_t)listener),
		/* Socket options are limited to SOL_SOCKET receive/send timeouts;
		 * transport selection, routing and other options are setup-only. */
		NATIVE_EQUAL(SYS_setsockopt, 0, 11),
		NATIVE_LOAD(28), NATIVE_EQUAL(0, 0, 7),
		NATIVE_LOAD(24), NATIVE_EQUAL(SOL_SOCKET, 0, 5),
		NATIVE_LOAD(36), NATIVE_EQUAL(0, 0, 3),
		NATIVE_LOAD(32), NATIVE_EQUAL(SO_RCVTIMEO, 2, 0),
		NATIVE_EQUAL(SO_SNDTIMEO, 1, 0), NATIVE_RETURN(NATIVE_DENY), NATIVE_RETURN(NATIVE_ALLOW),
		/* Allocator mappings must be anonymous and non-executable. File maps
		 * and later mprotect are denied, as are exec and process creation. */
		NATIVE_EQUAL(SYS_mmap, 0, 10),
		NATIVE_LOAD(36), NATIVE_EQUAL(0, 0, 7),
		NATIVE_LOAD(32), {0x45, 5, 0, PROT_EXEC},
		NATIVE_LOAD(40), {0x45, 0, 3, MAP_ANONYMOUS},
		NATIVE_LOAD(48), NATIVE_EQUAL(UINT_MAX, 0, 1),
		NATIVE_RETURN(NATIVE_ALLOW), NATIVE_RETURN(NATIVE_DENY),
		/* Branches above load arguments; restore nr before the ordinary
		 * allowlist. Every unmatched syscall returns EPERM. */
		NATIVE_LOAD(0),
		NATIVE_ALLOW_CALL(read), NATIVE_ALLOW_CALL(write), NATIVE_ALLOW_CALL(writev),
		NATIVE_ALLOW_CALL(close), NATIVE_ALLOW_CALL(recvfrom), NATIVE_ALLOW_CALL(sendto),
		NATIVE_ALLOW_CALL(shutdown), NATIVE_ALLOW_CALL(clock_gettime),
#ifdef SYS_poll
		NATIVE_ALLOW_CALL(poll),
#endif
		NATIVE_ALLOW_CALL(ppoll), NATIVE_ALLOW_CALL(rt_sigreturn),
		NATIVE_ALLOW_CALL(rt_sigprocmask), NATIVE_ALLOW_CALL(exit), NATIVE_ALLOW_CALL(exit_group),
		NATIVE_ALLOW_CALL(brk), NATIVE_ALLOW_CALL(munmap), NATIVE_ALLOW_CALL(madvise),
#ifdef SYS_fstat
		NATIVE_ALLOW_CALL(fstat),
#endif
		/* libc stdio may inspect existing stream metadata. This admits
		 * metadata lookup only; open/readlink/path mutations remain denied. */
		NATIVE_ALLOW_CALL(newfstatat),
		NATIVE_RETURN(NATIVE_DENY),
	};
	struct native_filter_program program = {
		.len = (uint16_t)(sizeof(filter) / sizeof(filter[0])), .filter = filter,
	};
	if (prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0) != 1) {
		errno = EPERM;
		return -1;
	}
	return prctl(PR_SET_SECCOMP, 2, &program, 0, 0);
}

#endif

#ifndef MULTIKERNEL_NATIVE_PRIVILEGES_H
#define MULTIKERNEL_NATIVE_PRIVILEGES_H

#include <errno.h>
#include <stdint.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <unistd.h>

/* Linux capability v3 UAPI: two words cover the 64-bit capability namespace.
 * Keep this independent of glibc-only headers for the static musl helpers. */
struct native_cap_header { uint32_t version; int32_t pid; };
struct native_cap_data { uint32_t effective, permitted, inheritable; };

#ifndef PR_CAP_AMBIENT
#define PR_CAP_AMBIENT 47
#define PR_CAP_AMBIENT_CLEAR_ALL 4
#endif

static int native_drop_request_privileges(void)
{
	struct native_cap_header header = { .version = 0x20080522, .pid = 0 };
	struct native_cap_data data[2] = { {0}, {0} };
	/* Set the irreversible exec barrier before clearing capabilities. The
	 * bounding set can stay populated when CAP_SETPCAP is unavailable; NNP
	 * prevents root exec or file capabilities from restoring the dropped set. */
	if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0 ||
	    prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0) < 0 ||
	    syscall(SYS_capset, &header, data) < 0 ||
	    syscall(SYS_capget, &header, data) < 0)
		return -1;
	for (unsigned int i = 0; i < 2; i++) {
		if (data[i].effective || data[i].permitted || data[i].inheritable) {
			errno = EPERM;
			return -1;
		}
	}
	if (prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0) != 1) {
		errno = EPERM;
		return -1;
	}
	return 0;
}

#endif

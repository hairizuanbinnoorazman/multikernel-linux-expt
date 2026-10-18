#ifndef MULTIKERNEL_NATIVE_SERVER_LIMITS_H
#define MULTIKERNEL_NATIVE_SERVER_LIMITS_H

#include <stdint.h>
#include <stdio.h>
#include <sys/resource.h>

#define NATIVE_SERVER_ADDRESS_LIMIT (64U * 1024U * 1024U)
#define NATIVE_SERVER_DESCRIPTOR_LIMIT 16U
#define NATIVE_SERVER_REFUSAL_DETAILS 16U

/* Lower both soft and hard limits without raising an inherited restriction.
 * Do not limit file size: the held image and diagnostic log share that limit. */
static int native_server_lower_limit(int resource, rlim_t maximum)
{
	struct rlimit limit;
	if (getrlimit(resource, &limit) < 0)
		return -1;
	if (limit.rlim_cur > maximum)
		limit.rlim_cur = maximum;
	if (limit.rlim_max > maximum)
		limit.rlim_max = maximum;
	return setrlimit(resource, &limit);
}

static int native_server_set_limits(void)
{
	return native_server_lower_limit(RLIMIT_CORE, 0) < 0 ||
		native_server_lower_limit(RLIMIT_NOFILE, NATIVE_SERVER_DESCRIPTOR_LIMIT) < 0 ||
		native_server_lower_limit(RLIMIT_AS, NATIVE_SERVER_ADDRESS_LIMIT) < 0 ? -1 : 0;
}

struct native_refusal_log { uint64_t total; unsigned int details; };

static void native_server_log_refusal(struct native_refusal_log *log, int identity)
{
	if (log->total != UINT64_MAX)
		log->total++;
	if (log->details < NATIVE_SERVER_REFUSAL_DETAILS) {
		fprintf(stderr, "MKNBD_SERVER_REFUSED reason=%s\n", identity ? "identity" : "handshake-io");
		log->details++;
	} else if (log->total == NATIVE_SERVER_REFUSAL_DETAILS + 1U) {
		fprintf(stderr, "MKNBD_SERVER_REFUSALS_SUPPRESSED further_details=omitted\n");
	}
}

static void native_server_log_refusal_summary(const struct native_refusal_log *log)
{
	if (log->total)
		printf("MKNBD_SERVER_REFUSAL_SUMMARY total=%llu details=%u suppressed=%llu\n",
		       (unsigned long long)log->total, log->details,
		       (unsigned long long)(log->total - log->details));
}

#endif

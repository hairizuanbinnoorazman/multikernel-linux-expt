#include <assert.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/file.h>
#include <sys/stat.h>
#include "../tools/native_privileges.h"

static void assert_zero(void)
{
	struct native_cap_header header = { .version = 0x20080522, .pid = 0 };
	struct native_cap_data data[2];
	assert(syscall(SYS_capget, &header, data) == 0);
	for (unsigned int i = 0; i < 2; i++)
		assert(!data[i].effective && !data[i].permitted && !data[i].inheritable);
	assert(prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0) == 1);
}

int main(int argc, char **argv)
{
	int fd;
	if (argc == 3 && !strcmp(argv[1], "--after-exec")) {
		fd = atoi(argv[2]);
		assert_zero();
		assert(pwrite(fd, "after-exec", 10, 0) == 10);
		assert(fsync(fd) == 0);
		char value[10];
		assert(pread(fd, value, sizeof(value), 0) == sizeof(value));
		assert(!memcmp(value, "after-exec", sizeof(value)));
		assert(flock(fd, LOCK_EX | LOCK_NB) == 0);
		puts("ZERO_CAPS_NO_NEW_PRIVS_EXEC_AND_HELD_IMAGE_PASS");
		return 0;
	}
	assert(argc == 2 || argc == 3);
	struct native_cap_header before_header = { .version = 0x20080522, .pid = 0 };
	struct native_cap_data before[2];
	assert(syscall(SYS_capget, &before_header, before) == 0);
	uint64_t initial = ((uint64_t)before[1].permitted << 32) | before[0].permitted;
	if (argc == 3)
		assert(initial == strtoull(argv[2], NULL, 16));
	printf("INITIAL_PERMITTED_MASK=%llx\n", (unsigned long long)initial);
	fflush(stdout);
	fd = open(argv[1], O_RDWR | O_CREAT | O_EXCL, 0600);
	assert(fd >= 0);
	assert(flock(fd, LOCK_EX | LOCK_NB) == 0);
	assert(ftruncate(fd, 4096) == 0);
	assert(native_drop_request_privileges() == 0);
	assert_zero();
	char descriptor[24];
	snprintf(descriptor, sizeof(descriptor), "%d", fd);
	execl(argv[0], argv[0], "--after-exec", descriptor, (char *)NULL);
	perror("exec privilege fixture");
	return 1;
}

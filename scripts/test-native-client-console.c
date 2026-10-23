// SPDX-License-Identifier: GPL-2.0-or-later
#define main native_helper_main
#include "../tools/mkvsock-nbd.c"
#undef main

int main(void)
{
	char path[] = "/tmp/mknbd-console-test-XXXXXX";
	int file = mkstemp(path);
	if (file < 0 || unlink(path) < 0)
		return 1;
	int saved = dup(STDERR_FILENO);
	struct stat before, after;
	if (saved < 0 || fstat(STDERR_FILENO, &before) < 0)
		return 2;
	errno = 0;
	if (client_console_fd(file) != -1 || errno != EPERM)
		return 3;
	int null = open("/dev/null", O_WRONLY | O_CLOEXEC);
	if (null < 0 || client_console_fd(null) != -1 || errno != EPERM)
		return 4;
	if (fstat(STDERR_FILENO, &after) < 0 || before.st_dev != after.st_dev || before.st_ino != after.st_ino)
		return 5;
	if (lseek(file, 0, SEEK_END) != 0)
		return 6;
	close(file); close(null); close(saved);
	puts("NATIVE_CLIENT_CONSOLE_WRONG_FILE_AND_DEVICE_REFUSED");
	return 0;
}

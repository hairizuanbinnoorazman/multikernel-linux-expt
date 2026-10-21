/* Execute real helper entrypoints, stopping before any external socket I/O. */
#define _GNU_SOURCE
#include <errno.h>
#include <poll.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/wait.h>
#include <unistd.h>
#include "../tools/native_privileges.h"

static int protected_prctl(int option, ...) __attribute__((unused));
static int protected_socket(int domain, int type, int protocol);
#define prctl protected_prctl
#define socket protected_socket
#define main helper_program_main
#ifndef SOURCE_FILE
#ifdef TEST_RELAY
#define SOURCE_FILE "../tools/mkvsock-relay.c"
#else
#define SOURCE_FILE "../tools/mkvsock-nbd.c"
#endif
#endif
#include SOURCE_FILE
#undef main
#undef socket
#undef prctl

static int fault, ready_fd, release_fd;
static int protected_prctl(int option, ...)
{
	va_list args;
	va_start(args, option);
	/* Only entry dumpability calls are reached: the first socket is replaced
	 * by a blocking observation point, before server/client setup continues. */
	if (option != PR_SET_DUMPABLE && option != PR_GET_DUMPABLE) abort();
	unsigned long a = (unsigned long)va_arg(args, int);
	unsigned long b = (unsigned long)va_arg(args, int);
	unsigned long c = (unsigned long)va_arg(args, int);
	unsigned long d = (unsigned long)va_arg(args, int);
	va_end(args);
	if (option == PR_SET_DUMPABLE && fault == 1) {
		errno = EPERM;
		return -1;
	}
	if (option == PR_GET_DUMPABLE && fault == 2)
		return 1;
	return (int)syscall(SYS_prctl, option, a, b, c, d);
}
static int protected_socket(int domain, int type, int protocol)
{
	(void)domain; (void)type; (void)protocol;
	if (fault)
		_exit(77); /* An establishment/readback failure must precede I/O. */
	char dumpable = (char)prctl(PR_GET_DUMPABLE, 0, 0, 0, 0);
	if (write(ready_fd, &dumpable, 1) != 1)
		_exit(78);
	char release;
	if (read(release_fd, &release, 1) != 1)
		_exit(79);
	_exit(0);
}
static int observe(pid_t target, int deny)
{
	pid_t child = fork();
	if (child < 0) return 1;
	if (child == 0) {
		if (native_drop_request_privileges() < 0) _exit(80);
		char path[64], buffer[4096];
		snprintf(path, sizeof(path), "/proc/%ld/root", (long)target);
		ssize_t n = readlink(path, buffer, sizeof(buffer));
		_exit(deny ? !(n < 0 && errno == EACCES) : !(n > 0));
	}
	int status;
	return waitpid(child, &status, 0) != child || !WIFEXITED(status) || WEXITSTATUS(status);
}
static int check(int inject)
{
	int ready[2], release[2], status;
	if (pipe(ready) || pipe(release)) return 1;
	pid_t child = fork();
	if (child < 0) return 1;
	if (child == 0) {
		close(ready[0]); close(release[1]);
		ready_fd=ready[1]; release_fd=release[0]; fault=inject;
		if (prctl(PR_SET_DUMPABLE, 1, 0, 0, 0) < 0) _exit(81);
#ifdef TEST_RELAY
		char *args[]={"relay","client","0","7921","/tmp/unused-control",NULL};
		_exit(helper_program_main(5,args));
#else
		char *args[]={"nbd","client","0","7921","/dev/nbd0","4096","image-a","generation-a","reserved",NULL};
		_exit(helper_program_main(9,args));
#endif
	}
	close(ready[1]);close(release[0]);
	int failed=0;
	if (!inject) {
		struct pollfd poller={.fd=ready[0],.events=POLLIN};
		char value=9;
		if (poll(&poller,1,2000)!=1 || read(ready[0],&value,1)!=1) failed=1;
#ifdef BASELINE_ALLOWED
		if (value!=1 || observe(child,0)) failed=1;
#else
		if (value!=0 || observe(child,1)) failed=1;
#endif
		if (write(release[1],"x",1)!=1) failed=1;
	}
	close(ready[0]);close(release[1]);
	if (waitpid(child,&status,0)!=child || !WIFEXITED(status) || WEXITSTATUS(status)!=(inject?1:0)) failed=1;
	return failed;
}
int main(void)
{
	if (check(0)) {fputs("procfs protection check failed\n",stderr);return 1;}
#ifdef BASELINE_ALLOWED
	puts("NATIVE_BASELINE_SAME_UID_PROC_ROOT_EXPOSED");
#else
	if (check(1) || check(2)) {fputs("protection failure admitted endpoint I/O\n",stderr);return 1;}
	puts("NATIVE_SAME_UID_PROC_ROOT_REFUSED_AND_FAIL_CLOSED");
#endif
	return 0;
}

#define main mkvsock_nbd_program_main
#include "../tools/mkvsock-nbd.c"
#undef main

static int expect_timeout(int descriptor, int option, long seconds)
{
	struct timeval timeout = { 0 };
	socklen_t length = sizeof(timeout);

	if (getsockopt(descriptor, SOL_SOCKET, option, &timeout, &length) < 0) {
		perror("getsockopt(timeout)");
		return 1;
	}
	if (length != sizeof(timeout) || timeout.tv_sec != seconds || timeout.tv_usec != 0) {
		fprintf(stderr, "timeout option %d = %ld.%06ld, want %ld.000000\n",
			option, (long)timeout.tv_sec, (long)timeout.tv_usec, seconds);
		return 1;
	}
	return 0;
}

static int test_hello_identity(void)
{
	const uint64_t size = 2147483648ULL;
	const char *image = "image-a";
	const char *generation = "0123456789abcdef0123456789abcdef";
	struct hello_wire hello;

	fill_hello(&hello, size, image, generation);
	if (!hello_matches(&hello, size, image, generation)) {
		fprintf(stderr, "exact hello identity did not match\n");
		return 1;
	}
	if (hello_matches(&hello, size + 4096, image, generation) ||
	    hello_matches(&hello, size, "image-b", generation) ||
	    hello_matches(&hello, size, image,
			  "fedcba9876543210fedcba9876543210")) {
		fprintf(stderr, "mismatched hello identity was accepted\n");
		return 1;
	}
	hello.magic = htonl(HELLO_MAGIC ^ 1U);
	if (hello_matches(&hello, size, image, generation)) {
		fprintf(stderr, "wrong hello magic was accepted\n");
		return 1;
	}
	fill_hello(&hello, size, image, generation);
	hello.version = htonl(HELLO_VERSION + 1U);
	if (hello_matches(&hello, size, image, generation)) {
		fprintf(stderr, "wrong hello version was accepted\n");
		return 1;
	}
	puts("MKVSOCK_NBD_HELLO_IDENTITY_PASS");
	return 0;
}

int main(void)
{
	int sockets[2];
	struct timeval probe = { .tv_sec = 15 };

	if (test_hello_identity())
		return 1;

	if (socketpair(AF_UNIX, SOCK_STREAM, 0, sockets) < 0)
		die("socketpair");
	if (setsockopt(sockets[0], SOL_SOCKET, SO_RCVTIMEO, &probe, sizeof(probe)) < 0) {
		if (errno == EPERM) {
			close(sockets[0]);
			close(sockets[1]);
			puts("MKVSOCK_NBD_POST_HANDSHAKE_TIMEOUT_CLEAR_SKIP reason=EPERM");
			return 0;
		}
		die("setsockopt(timeout probe)");
	}
	set_socket_timeout(sockets[0]);
	if (expect_timeout(sockets[0], SO_RCVTIMEO, 15) ||
	    expect_timeout(sockets[0], SO_SNDTIMEO, 15))
		return 1;
	clear_socket_timeout(sockets[0]);
	if (expect_timeout(sockets[0], SO_RCVTIMEO, 0) ||
	    expect_timeout(sockets[0], SO_SNDTIMEO, 0))
		return 1;
	close(sockets[0]);
	close(sockets[1]);
	puts("MKVSOCK_NBD_POST_HANDSHAKE_TIMEOUT_CLEAR_PASS");
	return 0;
}

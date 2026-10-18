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
	fill_hello(&hello, size, image, generation);
	hello.reserved = htonl(1);
	if (hello_matches(&hello, size, image, generation))
		return 1;
	fill_hello(&hello, size, image, generation);
	hello.status = htonl(EACCES);
	if (hello_matches(&hello, size, image, generation))
		return 1;
	fill_hello(&hello, size, image, generation);
	hello.image_id[sizeof(hello.image_id)-1] = 'x';
	if (hello_matches(&hello, size, image, generation))
		return 1;
	fill_hello(&hello, size, image, generation);
	hello.generation[sizeof(hello.generation)-1] = 'x';
	if (hello_matches(&hello, size, image, generation))
		return 1;
	puts("MKVSOCK_NBD_HELLO_IDENTITY_PASS");
	return 0;
}

static int test_request_validation(void)
{
	const struct {
		uint32_t type, length;
		uint64_t offset, size;
		int valid;
	} cases[] = {
		{NBD_CMD_READ, 4096, 0, 8192, 1},
		{NBD_CMD_WRITE, 4096, 4096, 8192, 1},
		{NBD_CMD_WRITE | NBD_CMD_FLAG_FUA, 4096, 0, 8192, 1},
		{NBD_CMD_FLUSH, 0, 0, 8192, 1},
		{NBD_CMD_DISC, 0, 0, 8192, 1},
		{NBD_CMD_READ | NBD_CMD_FLAG_FUA, 4096, 0, 8192, 0},
		{NBD_CMD_WRITE | (1U << 17), 4096, 0, 8192, 0},
		{NBD_CMD_FLUSH | NBD_CMD_FLAG_FUA, 0, 0, 8192, 0},
		{NBD_CMD_FLUSH, 1, 0, 8192, 0},
		{NBD_CMD_DISC, 0, 1, 8192, 0},
		{99, 4096, 0, 8192, 0},
		{NBD_CMD_READ, 0, 0, 8192, 1},
		{NBD_CMD_WRITE, 0, 8192, 8192, 1},
		{NBD_CMD_WRITE, MAX_REQUEST + 1, 0, UINT64_MAX, 0},
		{NBD_CMD_READ, 4096, 4097, 8192, 0},
		{NBD_CMD_WRITE, 1, UINT64_MAX, 8192, 0},
		{NBD_CMD_READ, 2, UINT64_MAX - 1, UINT64_MAX, 0},
		{NBD_CMD_READ, MAX_REQUEST, 0, MAX_REQUEST, 1},
	};
	for (size_t i = 0; i < sizeof(cases)/sizeof(cases[0]); i++) {
		struct nbd_request_wire request = {
			.magic = htonl(NBD_REQUEST_MAGIC), .type = htonl(cases[i].type),
			.len = htonl(cases[i].length), .from = htobe64(cases[i].offset),
		};
		if (request_valid(&request, cases[i].size) != cases[i].valid) {
			fprintf(stderr, "request validation mismatch case=%zu\n", i);
			return 1;
		}
		request.magic ^= htonl(1);
		if (request_valid(&request, cases[i].size))
			return 1;
	}
	puts("MKVSOCK_NBD_REQUEST_VALIDATION_PASS");
	return 0;
}

static int test_active_frame_deadlines(void)
{
	int pair[2];
	struct nbd_request_wire request = {
		.magic = htonl(NBD_REQUEST_MAGIC), .type = htonl(NBD_CMD_READ),
		.len = htonl(4096),
	};
	struct nbd_request_wire observed;
	struct timespec deadline;
	if (socketpair(AF_UNIX, SOCK_STREAM, 0, pair) < 0)
		die("socketpair(frame)");
	if (send(pair[1], "x", 1, MSG_NOSIGNAL) < 0) {
		if (errno == EPERM) {
			close(pair[0]); close(pair[1]);
			puts("MKVSOCK_NBD_ACTIVE_FRAME_DEADLINES_SKIP reason=EPERM");
			return 0;
		}
		die("send(frame probe)");
	}
	uint8_t probe;
	if (read_exact_or_eof(pair[0], &probe, 1) != 1)
		return 1;
	pid_t sender = fork();
	if (sender < 0)
		die("fork(frame)");
	if (!sender) {
		close(pair[0]);
		usleep(80000); /* Exceed the frame allowance while legitimately idle. */
		xwrite(pair[1], &request, sizeof(request));
		close(pair[1]);
		_exit(0);
	}
	close(pair[1]);
	int rc = read_request(pair[0], &observed, &deadline, 40);
	close(pair[0]);
	int status;
	if (waitpid(sender, &status, 0) != sender || !WIFEXITED(status) || WEXITSTATUS(status) ||
	    rc != 1 || memcmp(&request, &observed, sizeof(request))) {
		fprintf(stderr, "idle connection did not accept complete request\n");
		return 1;
	}
	if (socketpair(AF_UNIX, SOCK_STREAM, 0, pair) < 0)
		die("socketpair(trickle)");
	sender = fork();
	if (sender < 0)
		die("fork(trickle)");
	if (!sender) {
		close(pair[0]);
		for (size_t i = 0; i < sizeof(request); i++) {
			if (send(pair[1], (uint8_t *)&request + i, 1, MSG_NOSIGNAL) != 1)
				break;
			usleep(10000);
		}
		close(pair[1]);
		_exit(0);
	}
	close(pair[1]);
	rc = read_request(pair[0], &observed, &deadline, 40);
	int saved_errno = errno;
	close(pair[0]);
	if (waitpid(sender, &status, 0) != sender || rc != -1 || saved_errno != ETIMEDOUT) {
		fprintf(stderr, "trickled header did not hit absolute deadline rc=%d errno=%d status=%d\n", rc, saved_errno, status);
		return 1;
	}
	if (socketpair(AF_UNIX, SOCK_STREAM, 0, pair) < 0)
		die("socketpair(payload)");
	xwrite(pair[1], &request, sizeof(request));
	if (read_request(pair[0], &observed, &deadline, 40) != 1)
		return 1;
	uint8_t payload[2];
	xwrite(pair[1], "x", 1);
	if (transfer_until(pair[0], payload, sizeof(payload), 0, &deadline) != -1 || errno != ETIMEDOUT) {
		fprintf(stderr, "partial write payload did not hit shared deadline\n");
		return 1;
	}
	close(pair[0]); close(pair[1]);
	if (socketpair(AF_UNIX, SOCK_STREAM, 0, pair) < 0)
		die("socketpair(response)");
	uint8_t *response = calloc(1, 8U * 1024U * 1024U);
	if (!response)
		die("calloc(response)");
	deadline = frame_deadline(40);
	rc = transfer_until(pair[0], response, 8U * 1024U * 1024U, 1, &deadline);
	saved_errno = errno;
	free(response);
	close(pair[0]); close(pair[1]);
	if (rc != -1 || saved_errno != ETIMEDOUT) {
		fprintf(stderr, "unread response did not hit absolute deadline\n");
		return 1;
	}
	puts("MKVSOCK_NBD_ACTIVE_FRAME_DEADLINES_PASS");
	return 0;
}

static int test_server_handshake(void)
{
	const char *image = "synthetic-image";
	const char *generation = "0123456789abcdef0123456789abcdef";
	const uint64_t size = 4096;
	for (int mode = 0; mode < 6; mode++) {
		int sockets[2];
		struct hello_wire hello, reply;
		if (socketpair(AF_UNIX, SOCK_STREAM, 0, sockets) < 0)
			die("handshake socketpair");
		fill_hello(&hello, size, image, generation);
		if (mode == 1)
			hello.reserved = htonl(1);
		if (mode == 2)
			fill_hello(&hello, size, "wrong-image", generation);
		if (mode == 3)
			fill_hello(&hello, size, image, "wrong-generation");
		if (mode != 5) {
			size_t length = mode == 4 ? 1 : sizeof(hello);
			if (send(sockets[1], &hello, length, MSG_NOSIGNAL) != (ssize_t)length) {
				int saved = errno;
				close(sockets[0]); close(sockets[1]);
				if (saved == EPERM) {
					puts("MKVSOCK_NBD_HANDSHAKE_SKIP reason=EPERM");
					return 0;
				}
				return 1;
			}
		}
		int rc = server_handshake(sockets[0], size, image, generation, 40);
		int saved = errno;
		if (mode < 4) {
			struct hello_wire expected;
			fill_hello(&expected, mode == 0 ? size : 0,
				   mode == 0 ? image : "", mode == 0 ? generation : "");
			expected.status = htonl(mode == 0 ? 0 : EACCES);
			if (rc != (mode == 0 ? 1 : 0) ||
			    read_exact_or_eof(sockets[1], &reply, sizeof(reply)) != 1 ||
			    memcmp(&reply, &expected, sizeof(reply))) {
				close(sockets[0]); close(sockets[1]);
				return 1;
			}
		} else if (rc != -1 || saved != ETIMEDOUT) {
			close(sockets[0]); close(sockets[1]);
			return 1;
		}
		close(sockets[0]); close(sockets[1]);
	}
	puts("MKVSOCK_NBD_HANDSHAKE_PASS");
	return 0;
}

int main(void)
{
	int sockets[2];
	struct timeval probe = { .tv_sec = 15 };

	if (test_hello_identity() || test_request_validation() || test_active_frame_deadlines() || test_server_handshake())
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

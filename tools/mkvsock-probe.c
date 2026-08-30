// SPDX-License-Identifier: GPL-2.0-or-later
#include <arpa/inet.h>
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <unistd.h>

#ifndef AF_VSOCK
#define AF_VSOCK 40
#endif
#define VMADDR_CID_ANY 0xffffffffU
struct sockaddr_vm {
	unsigned short svm_family;
	unsigned short svm_reserved1;
	unsigned int svm_port;
	unsigned int svm_cid;
	unsigned char svm_flags;
	unsigned char svm_zero[3];
};

#ifndef SO_VM_SOCKETS_TRANSPORT
#define SO_VM_SOCKETS_TRANSPORT 9
#endif
#ifndef VSOCK_TRANSPORT_MULTIKERNEL
#define VSOCK_TRANSPORT_MULTIKERNEL 1
#endif

static void die(const char *what)
{
	perror(what);
	exit(1);
}

static void set_transport(int fd)
{
	int transport = VSOCK_TRANSPORT_MULTIKERNEL;
	struct timeval timeout = { .tv_sec = 8 };

	if (setsockopt(fd, AF_VSOCK, SO_VM_SOCKETS_TRANSPORT,
		       &transport, sizeof(transport)) < 0)
		die("setsockopt(SO_VM_SOCKETS_TRANSPORT)");
	if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout)) < 0)
		die("setsockopt(SO_RCVTIMEO)");
	if (setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof(timeout)) < 0)
		die("setsockopt(SO_SNDTIMEO)");
}

static void xwrite(int fd, const void *buf, size_t len)
{
	const unsigned char *p = buf;

	while (len) {
		ssize_t n = write(fd, p, len);
		if (n < 0 && errno == EINTR)
			continue;
		if (n <= 0)
			die("write");
		p += n;
		len -= (size_t)n;
	}
}

static void xread(int fd, void *buf, size_t len)
{
	unsigned char *p = buf;

	while (len) {
		ssize_t n = read(fd, p, len);
		if (n < 0 && errno == EINTR)
			continue;
		if (n <= 0) {
			if (!n)
				errno = ECONNRESET;
			die("read");
		}
		p += n;
		len -= (size_t)n;
	}
}

static unsigned char pattern(size_t offset, uint32_t sequence)
{
	return (unsigned char)((offset * 131U + sequence * 17U + 29U) & 0xffU);
}

static void verify_pattern(const unsigned char *buf, size_t len, uint32_t sequence)
{
	for (size_t i = 0; i < len; i++) {
		if (buf[i] != pattern(i, sequence)) {
			fprintf(stderr, "pattern mismatch sequence=%u offset=%zu\n",
				sequence, i);
			exit(1);
		}
	}
}

static int make_socket(void)
{
	int fd = socket(AF_VSOCK, SOCK_STREAM, 0);
	if (fd < 0)
		die("socket(AF_VSOCK)");
	set_transport(fd);
	return fd;
}

static void run_server(unsigned int port)
{
	struct sockaddr_vm addr = {
		.svm_family = AF_VSOCK,
		.svm_cid = VMADDR_CID_ANY,
		.svm_port = port,
	};
	int listener = make_socket();
	unsigned char *buf;

	if (bind(listener, (struct sockaddr *)&addr, sizeof(addr)) < 0)
		die("bind");
	if (listen(listener, 1) < 0)
		die("listen");
	printf("MKVSOCK_SERVER_READY port=%u\n", port);
	fflush(stdout);

	int client = accept(listener, NULL, NULL);
	if (client < 0)
		die("accept");
	buf = malloc(1024 * 1024);
	if (!buf)
		die("malloc");

	for (;;) {
		uint32_t header[2];
		xread(client, header, sizeof(header));
		uint32_t len = ntohl(header[0]);
		uint32_t sequence = ntohl(header[1]);
		if (!len)
			break;
		if (len > 1024 * 1024) {
			fprintf(stderr, "oversized frame: %u\n", len);
			exit(1);
		}
		xread(client, buf, len);
		verify_pattern(buf, len, sequence);
		xwrite(client, header, sizeof(header));
		xwrite(client, buf, len);
	}

	printf("MKVSOCK_SERVER_PASS\n");
	free(buf);
	close(client);
	close(listener);
}

static void run_client(unsigned int cid, unsigned int port)
{
	static const uint32_t sizes[] = {
		1, 63, 64, 4095, 4096, 4097, 16384, 32768, 65536, 1048576
	};
	struct sockaddr_vm addr = {
		.svm_family = AF_VSOCK,
		.svm_cid = cid,
		.svm_port = port,
	};
	unsigned char *sent = malloc(1024 * 1024);
	unsigned char *received = malloc(1024 * 1024);
	int fd = make_socket();

	if (!sent || !received)
		die("malloc");
	if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0)
		die("connect");

	for (uint32_t sequence = 0;
	     sequence < sizeof(sizes) / sizeof(sizes[0]); sequence++) {
		uint32_t len = sizes[sequence];
		uint32_t header[2] = { htonl(len), htonl(sequence) };
		uint32_t reply[2];

		for (size_t i = 0; i < len; i++)
			sent[i] = pattern(i, sequence);
		xwrite(fd, header, sizeof(header));
		xwrite(fd, sent, len);
		xread(fd, reply, sizeof(reply));
		if (memcmp(header, reply, sizeof(header))) {
			fprintf(stderr, "header mismatch sequence=%u\n", sequence);
			exit(1);
		}
		xread(fd, received, len);
		if (memcmp(sent, received, len)) {
			fprintf(stderr, "echo mismatch sequence=%u length=%u\n",
				sequence, len);
			exit(1);
		}
		printf("MKVSOCK_CASE_PASS sequence=%u bytes=%u\n", sequence, len);
	}

	uint32_t done[2] = { 0, 0 };
	xwrite(fd, done, sizeof(done));
	printf("MKVSOCK_CLIENT_PASS\n");
	free(received);
	free(sent);
	close(fd);
}

int main(int argc, char **argv)
{
	if (argc == 3 && !strcmp(argv[1], "server")) {
		run_server((unsigned int)strtoul(argv[2], NULL, 10));
		return 0;
	}
	if (argc == 4 && !strcmp(argv[1], "client")) {
		run_client((unsigned int)strtoul(argv[2], NULL, 10),
			   (unsigned int)strtoul(argv[3], NULL, 10));
		return 0;
	}
	fprintf(stderr, "usage: %s server PORT | client CID PORT\n", argv[0]);
	return 2;
}

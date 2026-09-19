// SPDX-License-Identifier: GPL-2.0-or-later
#include <arpa/inet.h>
#include <endian.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <poll.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/file.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/wait.h>
#include <unistd.h>

#ifndef AF_VSOCK
#define AF_VSOCK 40
#endif
#define VMADDR_CID_ANY 0xffffffffU
#define SO_VM_SOCKETS_TRANSPORT 9
#define VSOCK_TRANSPORT_MULTIKERNEL 1

#define NBD_SET_SOCK _IO(0xab, 0)
#define NBD_SET_BLKSIZE _IO(0xab, 1)
#define NBD_DO_IT _IO(0xab, 3)
#define NBD_CLEAR_SOCK _IO(0xab, 4)
#define NBD_CLEAR_QUE _IO(0xab, 5)
#define NBD_SET_SIZE_BLOCKS _IO(0xab, 7)
#define NBD_DISCONNECT _IO(0xab, 8)
#define NBD_SET_TIMEOUT _IO(0xab, 9)
#define NBD_SET_FLAGS _IO(0xab, 10)
#define NBD_FLAG_HAS_FLAGS (1U << 0)
#define NBD_FLAG_SEND_FLUSH (1U << 2)
#define NBD_FLAG_SEND_FUA (1U << 3)
#define NBD_CMD_READ 0
#define NBD_CMD_WRITE 1
#define NBD_CMD_DISC 2
#define NBD_CMD_FLUSH 3
#define NBD_CMD_MASK_COMMAND 0xffffU
#define NBD_CMD_FLAG_FUA (1U << 16)
#define NBD_REQUEST_MAGIC 0x25609513U
#define NBD_REPLY_MAGIC 0x67446698U
#define HELLO_MAGIC 0x4d4b4e42U
#define HELLO_VERSION 1U
#define MAX_REQUEST (4U * 1024U * 1024U)

static volatile sig_atomic_t stop_requested;
static int active_client = -1;
static int active_listener = -1;

static void request_stop(int signal_number)
{
	(void)signal_number;
	stop_requested = 1;
	if (active_client >= 0)
		shutdown(active_client, SHUT_RDWR);
	if (active_listener >= 0)
		shutdown(active_listener, SHUT_RDWR);
}

struct sockaddr_vm {
	unsigned short svm_family;
	unsigned short svm_reserved1;
	unsigned int svm_port;
	unsigned int svm_cid;
	unsigned char svm_flags;
	unsigned char svm_zero[3];
};

struct nbd_request_wire {
	uint32_t magic;
	uint32_t type;
	uint8_t handle[8];
	uint64_t from;
	uint32_t len;
} __attribute__((packed));

struct nbd_reply_wire {
	uint32_t magic;
	uint32_t error;
	uint8_t handle[8];
} __attribute__((packed));

struct hello_wire {
	uint32_t magic;
	uint32_t version;
	uint32_t status;
	uint32_t reserved;
	uint64_t size;
	char image_id[64];
	char generation[64];
} __attribute__((packed));

static void die(const char *what)
{
	perror(what);
	exit(1);
}

static void xwrite(int fd, const void *buf, size_t len)
{
	const uint8_t *p = buf;
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

static int read_exact_or_eof(int fd, void *buf, size_t len)
{
	uint8_t *p = buf;
	while (len) {
		ssize_t n = read(fd, p, len);
		if (n < 0 && errno == EINTR)
			continue;
		if (!n)
			return 0;
		if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK))
			return -1;
		if (n < 0)
			die("read");
		p += n;
		len -= (size_t)n;
	}
	return 1;
}

static void set_socket_timeout(int fd)
{
	struct timeval timeout = { .tv_sec = 15 };
	if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout)) < 0)
		die("setsockopt(rcvtimeo)");
	if (setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof(timeout)) < 0)
		die("setsockopt(sndtimeo)");
}

static void set_transport(int fd)
{
	int transport = VSOCK_TRANSPORT_MULTIKERNEL;
	if (setsockopt(fd, AF_VSOCK, SO_VM_SOCKETS_TRANSPORT,
		       &transport, sizeof(transport)) < 0)
		die("setsockopt(transport)");
	set_socket_timeout(fd);
}

static void forward_stream(int left, int right)
{
	uint8_t buffer[65536];
	struct pollfd fds[2] = {
		{ .fd = left, .events = POLLIN },
		{ .fd = right, .events = POLLIN },
	};
	for (;;) {
		int rc = poll(fds, 2, -1);
		if (rc < 0 && errno == EINTR)
			continue;
		if (rc < 0)
			die("poll(proxy)");
		for (int i = 0; i < 2; i++) {
			if (!(fds[i].revents & (POLLIN | POLLHUP | POLLERR)))
				continue;
			int source = i ? right : left;
			int destination = i ? left : right;
			ssize_t n = read(source, buffer, sizeof(buffer));
			if (n < 0 && errno == EINTR)
				continue;
			if (n <= 0) {
				shutdown(destination, SHUT_WR);
				return;
			}
			xwrite(destination, buffer, (size_t)n);
		}
	}
}

static int make_nbd_tcp_socket(int vsock, pid_t *proxy_pid)
{
	struct sockaddr_in addr = {
		.sin_family = AF_INET,
		.sin_addr.s_addr = htonl(INADDR_LOOPBACK),
		.sin_port = 0,
	};
	socklen_t addr_len = sizeof(addr);
	int listener = socket(AF_INET, SOCK_STREAM, 0);
	if (listener < 0)
		die("socket(tcp listener)");
	if (bind(listener, (struct sockaddr *)&addr, sizeof(addr)) < 0 ||
	    listen(listener, 1) < 0 ||
	    getsockname(listener, (struct sockaddr *)&addr, &addr_len) < 0)
		die("setup(tcp listener)");
	int client = socket(AF_INET, SOCK_STREAM, 0);
	if (client < 0 || connect(client, (struct sockaddr *)&addr, sizeof(addr)) < 0)
		die("connect(tcp loopback)");
	int server = accept(listener, NULL, NULL);
	if (server < 0)
		die("accept(tcp loopback)");

	pid_t pid = fork();
	if (pid < 0)
		die("fork(proxy)");
	if (!pid) {
		close(client);
		close(listener);
		forward_stream(server, vsock);
		close(server);
		close(vsock);
		_exit(0);
	}
	close(server);
	close(listener);
	close(vsock);
	*proxy_pid = pid;
	return client;
}

static int vsock_socket(void)
{
	int fd = socket(AF_VSOCK, SOCK_STREAM, 0);
	if (fd < 0)
		die("socket(AF_VSOCK)");
	set_transport(fd);
	return fd;
}

static void fill_hello(struct hello_wire *hello, uint64_t size,
		       const char *image_id, const char *generation)
{
	memset(hello, 0, sizeof(*hello));
	hello->magic = htonl(HELLO_MAGIC);
	hello->version = htonl(HELLO_VERSION);
	hello->size = htobe64(size);
	if (strlen(image_id) >= sizeof(hello->image_id) ||
	    strlen(generation) >= sizeof(hello->generation)) {
		errno = EINVAL;
		die("export identity too long");
	}
	strcpy(hello->image_id, image_id);
	strcpy(hello->generation, generation);
}

static int hello_matches(const struct hello_wire *hello, uint64_t size,
			 const char *image_id, const char *generation)
{
	return ntohl(hello->magic) == HELLO_MAGIC &&
		ntohl(hello->version) == HELLO_VERSION &&
		be64toh(hello->size) == size &&
		!strncmp(hello->image_id, image_id, sizeof(hello->image_id)) &&
		!strncmp(hello->generation, generation, sizeof(hello->generation));
}

static void send_reply(int sock, const struct nbd_request_wire *request, int error)
{
	struct nbd_reply_wire reply = {
		.magic = htonl(NBD_REPLY_MAGIC),
		.error = htonl((uint32_t)error),
	};
	memcpy(reply.handle, request->handle, sizeof(reply.handle));
	xwrite(sock, &reply, sizeof(reply));
}

static void run_server(const char *image, unsigned int port,
		       const char *image_id, const char *generation)
{
	struct sockaddr_vm addr = {
		.svm_family = AF_VSOCK,
		.svm_cid = VMADDR_CID_ANY,
		.svm_port = port,
	};
	struct stat st;
	struct hello_wire hello, response;
	uint64_t reads = 0, writes = 0, flushes = 0, bytes_read = 0, bytes_written = 0;
	uint8_t *buffer = malloc(MAX_REQUEST);
	/*
	 * mkruntimed passes an already validated and exclusively locked image as
	 * fd 3.  Duplicating it preserves the same open-file-description lock, so
	 * there is no unlock/reopen window between validation and serving.  Keep
	 * pathname mode for the standalone experiment scripts.
	 */
	int image_fd;
	if (!strcmp(image, "/proc/self/fd/3"))
		image_fd = fcntl(3, F_DUPFD_CLOEXEC, 4);
	else
		image_fd = open(image, O_RDWR | O_CLOEXEC);
	if (image_fd < 0)
		die("open(image)");
	if (flock(image_fd, LOCK_EX | LOCK_NB) < 0)
		die("flock(image)");
	if (fstat(image_fd, &st) < 0 || st.st_size <= 0)
		die("fstat(image)");
	if (!buffer)
		die("malloc");
	struct sigaction stop_action = { .sa_handler = request_stop };
	sigemptyset(&stop_action.sa_mask);
	if (sigaction(SIGTERM, &stop_action, NULL) < 0 ||
	    sigaction(SIGINT, &stop_action, NULL) < 0)
		die("sigaction");

	int listener = vsock_socket();
	active_listener = listener;
	if (bind(listener, (struct sockaddr *)&addr, sizeof(addr)) < 0)
		die("bind");
	if (listen(listener, 1) < 0)
		die("listen");
	struct timeval no_timeout = { 0 };
	if (setsockopt(listener, SOL_SOCKET, SO_RCVTIMEO,
		       &no_timeout, sizeof(no_timeout)) < 0)
		die("clear listener timeout");
	printf("MKNBD_SERVER_READY image=%s image_id=%s generation=%s size=%lld port=%u\n",
	       image, image_id, generation, (long long)st.st_size, port);
	fflush(stdout);
	int sock = accept(listener, NULL, NULL);
	if (sock < 0 && stop_requested)
		goto closed;
	if (sock < 0)
		die("accept");
	active_client = sock;
	set_socket_timeout(sock);
	if (read_exact_or_eof(sock, &hello, sizeof(hello)) <= 0)
		die("hello eof");
	fill_hello(&response, (uint64_t)st.st_size, image_id, generation);
	if (!hello_matches(&hello, (uint64_t)st.st_size, image_id, generation)) {
		response.status = htonl(EACCES);
		xwrite(sock, &response, sizeof(response));
		fprintf(stderr, "MKNBD_SERVER_REFUSED reason=identity\n");
		exit(2);
	}
	xwrite(sock, &response, sizeof(response));
	printf("MKNBD_SERVER_CLIENT_ACCEPTED\n");
	fflush(stdout);

	for (;;) {
		struct nbd_request_wire request;
		if (read_exact_or_eof(sock, &request, sizeof(request)) <= 0)
			break;
		uint32_t type = ntohl(request.type);
		uint32_t command = type & NBD_CMD_MASK_COMMAND;
		uint32_t len = ntohl(request.len);
		uint64_t offset = be64toh(request.from);
		int error = 0;

		if (ntohl(request.magic) != NBD_REQUEST_MAGIC || len > MAX_REQUEST ||
		    offset > (uint64_t)st.st_size || len > (uint64_t)st.st_size - offset) {
			fprintf(stderr, "MKNBD_SERVER_REFUSED reason=request command=%u len=%u offset=%llu\n",
				command, len, (unsigned long long)offset);
			exit(2);
		}
		if (command == NBD_CMD_DISC)
			break;
		if (command == NBD_CMD_WRITE) {
			if (read_exact_or_eof(sock, buffer, len) <= 0) {
				printf("MKNBD_SERVER_DISCONNECT_DURING_WRITE len=%u offset=%llu\n",
				       len, (unsigned long long)offset);
				break;
			}
			ssize_t n = pwrite(image_fd, buffer, len, (off_t)offset);
			if (n != (ssize_t)len)
				error = errno ? errno : EIO;
			else {
				writes++;
				bytes_written += len;
			}
			if (!error && (type & NBD_CMD_FLAG_FUA) && fdatasync(image_fd) < 0)
				error = errno;
			send_reply(sock, &request, error);
		} else if (command == NBD_CMD_READ) {
			ssize_t n = pread(image_fd, buffer, len, (off_t)offset);
			if (n != (ssize_t)len)
				error = errno ? errno : EIO;
			else {
				reads++;
				bytes_read += len;
			}
			send_reply(sock, &request, error);
			if (!error)
				xwrite(sock, buffer, len);
		} else if (command == NBD_CMD_FLUSH) {
			if (fdatasync(image_fd) < 0)
				error = errno;
			else
				flushes++;
			send_reply(sock, &request, error);
		} else {
			send_reply(sock, &request, EOPNOTSUPP);
		}
	}

closed:
	active_client = -1;
	active_listener = -1;
	if (fdatasync(image_fd) < 0)
		die("final fdatasync");
	printf("MKNBD_SERVER_CLOSED reads=%llu read_bytes=%llu writes=%llu write_bytes=%llu flushes=%llu\n",
	       (unsigned long long)reads, (unsigned long long)bytes_read,
	       (unsigned long long)writes, (unsigned long long)bytes_written,
	       (unsigned long long)flushes);
	free(buffer);
	if (sock >= 0)
		close(sock);
	close(listener);
	close(image_fd);
}

static void run_client(unsigned int cid, unsigned int port, const char *device,
		       uint64_t size, const char *image_id, const char *generation)
{
	struct sockaddr_vm addr = {
		.svm_family = AF_VSOCK,
		.svm_cid = cid,
		.svm_port = port,
	};
	struct hello_wire hello, response;
	int sock = vsock_socket();
	if (connect(sock, (struct sockaddr *)&addr, sizeof(addr)) < 0)
		die("connect");
	fill_hello(&hello, size, image_id, generation);
	xwrite(sock, &hello, sizeof(hello));
	if (read_exact_or_eof(sock, &response, sizeof(response)) <= 0)
		die("hello response eof");
	if (ntohl(response.status) || !hello_matches(&response, size, image_id, generation)) {
		fprintf(stderr, "MKNBD_CLIENT_REFUSED reason=identity status=%u\n",
			ntohl(response.status));
		exit(2);
	}
	printf("MKNBD_CLIENT_CONNECTED image_id=%s generation=%s size=%llu\n",
	       image_id, generation, (unsigned long long)size);
	fflush(stdout);
	pid_t proxy_pid;
	sock = make_nbd_tcp_socket(sock, &proxy_pid);

	int nbd = open(device, O_RDWR | O_CLOEXEC);
	if (nbd < 0)
		die("open(nbd)");
	if (ioctl(nbd, NBD_SET_SOCK, sock) < 0 ||
	    ioctl(nbd, NBD_SET_BLKSIZE, 4096UL) < 0 ||
	    ioctl(nbd, NBD_SET_SIZE_BLOCKS, (unsigned long)(size / 4096U)) < 0 ||
	    ioctl(nbd, NBD_SET_TIMEOUT, 15UL) < 0 ||
	    ioctl(nbd, NBD_SET_FLAGS, (unsigned long)(NBD_FLAG_HAS_FLAGS |
		NBD_FLAG_SEND_FLUSH | NBD_FLAG_SEND_FUA)) < 0)
		die("nbd setup ioctl");
	printf("MKNBD_CLIENT_DEVICE_READY device=%s\n", device);
	fflush(stdout);
	if (ioctl(nbd, NBD_DO_IT) < 0 && errno != EPIPE)
		perror("NBD_DO_IT");
	ioctl(nbd, NBD_CLEAR_QUE);
	ioctl(nbd, NBD_CLEAR_SOCK);
	close(nbd);
	close(sock);
	kill(proxy_pid, SIGTERM);
	waitpid(proxy_pid, NULL, 0);
}

static void disconnect_device(const char *device)
{
	int fd = open(device, O_RDWR | O_CLOEXEC);
	if (fd < 0)
		die("open(nbd disconnect)");
	if (ioctl(fd, NBD_DISCONNECT) < 0)
		die("NBD_DISCONNECT");
	close(fd);
}

static void print_ext4_uuid(const char *device)
{
	uint8_t uuid[16];
	int fd = open(device, O_RDONLY | O_CLOEXEC);
	if (fd < 0)
		die("open(uuid device)");
	if (pread(fd, uuid, sizeof(uuid), 1024 + 0x68) != (ssize_t)sizeof(uuid))
		die("pread(ext4 uuid)");
	close(fd);
	printf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-"
	       "%02x%02x%02x%02x%02x%02x\n",
	       uuid[0], uuid[1], uuid[2], uuid[3], uuid[4], uuid[5],
	       uuid[6], uuid[7], uuid[8], uuid[9], uuid[10], uuid[11],
	       uuid[12], uuid[13], uuid[14], uuid[15]);
}

int main(int argc, char **argv)
{
	if (argc == 6 && !strcmp(argv[1], "server")) {
		run_server(argv[2], (unsigned int)strtoul(argv[3], NULL, 10), argv[4], argv[5]);
		return 0;
	}
	if (argc == 9 && !strcmp(argv[1], "client")) {
		run_client((unsigned int)strtoul(argv[2], NULL, 10),
			   (unsigned int)strtoul(argv[3], NULL, 10), argv[4],
			   strtoull(argv[5], NULL, 10), argv[6], argv[7]);
		return 0;
	}
	if (argc == 3 && !strcmp(argv[1], "disconnect")) {
		disconnect_device(argv[2]);
		return 0;
	}
	if (argc == 3 && !strcmp(argv[1], "uuid")) {
		print_ext4_uuid(argv[2]);
		return 0;
	}
	fprintf(stderr, "usage: %s server IMAGE PORT IMAGE_ID GENERATION | client CID PORT DEVICE SIZE IMAGE_ID GENERATION RESERVED | disconnect DEVICE | uuid DEVICE\n", argv[0]);
	return 2;
}

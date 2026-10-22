// SPDX-License-Identifier: GPL-2.0-or-later
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include "../tools/native_stream_proxy.h"

/* Retain the prior read/whole-blocking-write shape as a failure control. */
static int blocking_control(int left, int right)
{
	unsigned char buffer[65536];
	struct pollfd fds[2] = {{.fd = left, .events = POLLIN}, {.fd = right, .events = POLLIN}};
	for (;;) {
		if (poll(fds, 2, -1) < 0)
			return -1;
		for (int i = 0; i < 2; i++) {
			if (!(fds[i].revents & (POLLIN | POLLHUP | POLLERR)))
				continue;
			ssize_t n = read(fds[i].fd, buffer, sizeof(buffer));
			if (n <= 0)
				return 0;
			size_t position = 0;
			while (position < (size_t)n) {
				ssize_t sent = send(fds[1-i].fd, buffer + position,
						    (size_t)n - position, MSG_NOSIGNAL);
				if (sent <= 0)
					return -1;
				position += (size_t)sent;
			}
		}
	}
}

int main(int argc, char **argv)
{
	if (argc != 4)
		return 2;
	int result = atoi(argv[1]) ? blocking_control(atoi(argv[2]), atoi(argv[3])) :
		native_stream_proxy(atoi(argv[2]), atoi(argv[3]));
	if (result < 0)
		perror("stream proxy");
	return result < 0;
}

// SPDX-License-Identifier: GPL-2.0-or-later
#ifndef MULTIKERNEL_NATIVE_STREAM_PROXY_H
#define MULTIKERNEL_NATIVE_STREAM_PROXY_H

#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#ifndef NATIVE_PROXY_TRACE
#define NATIVE_PROXY_TRACE(stage, endpoint, detail) ((void)0)
#endif

static int native_proxy_error(const char *stage, int endpoint, int error)
{
	(void)stage;
	(void)endpoint;
	NATIVE_PROXY_TRACE(stage, endpoint, error);
	errno = error;
	return -1;
}

/* NBD transmits requests and replies independently. A blocking write while
 * forwarding one direction can prevent draining the reply needed by the other
 * endpoint. Keep one bounded FIFO per direction and poll both concurrently. */
static int native_stream_proxy(int left, int right)
{
	struct direction {
		unsigned char data[65536];
		size_t begin, end;
		int eof, shutdown;
	} direction[2] = {0};
	int fd[2] = {left, right};
	for (int i = 0; i < 2; i++) {
		int flags = fcntl(fd[i], F_GETFL);
		if (flags < 0 || fcntl(fd[i], F_SETFL, flags | O_NONBLOCK) < 0)
			return native_proxy_error("fcntl", i, errno);
	}
	for (;;) {
		struct pollfd ready[2] = {{.fd = left}, {.fd = right}};
		int completed = 0;
		for (int i = 0; i < 2; i++) {
			struct direction *d = &direction[i];
			if (d->begin == d->end)
				d->begin = d->end = 0;
			else if (d->end == sizeof(d->data) && d->begin) {
				memmove(d->data, d->data + d->begin, d->end - d->begin);
				d->end -= d->begin;
				d->begin = 0;
			}
			if (d->eof && !d->end && !d->shutdown) {
				if (shutdown(fd[1-i], SHUT_WR) < 0 && errno != ENOTCONN)
					return native_proxy_error("shutdown", 1-i, errno);
				d->shutdown = 1;
			}
			if (d->shutdown)
				completed++;
			if (!d->eof && d->end < sizeof(d->data))
				ready[i].events |= POLLIN;
			if (d->end > d->begin)
				ready[1-i].events |= POLLOUT;
		}
		if (completed == 2) {
			NATIVE_PROXY_TRACE("complete", -1, 0);
			return 0;
		}
		/* Ignore a fully drained half so persistent HUP cannot cause a spin. */
		for (int i = 0; i < 2; i++)
			if (!ready[i].events)
				ready[i].fd = -1;
		int result = poll(ready, 2, -1);
		if (result < 0 && errno == EINTR)
			continue;
		if (result < 0)
			return native_proxy_error("poll", -1, errno);
		for (int i = 0; i < 2; i++) {
			if (ready[i].revents & POLLNVAL) {
				return native_proxy_error("poll-invalid", i, EBADF);
			}
			if (ready[i].revents & POLLERR) {
				int error = 0;
				socklen_t size = sizeof(error);
				if (getsockopt(fd[i], SOL_SOCKET, SO_ERROR, &error, &size) < 0)
					return native_proxy_error("socket-error-query", i, errno);
				NATIVE_PROXY_TRACE("poll-error-value", i, error);
				return native_proxy_error("poll-error", i, error ? error : EIO);
			}
		}
		for (int i = 0; i < 2; i++) {
			struct direction *d = &direction[i];
			if (d->end > d->begin && (ready[1-i].revents & (POLLOUT | POLLHUP))) {
				ssize_t n = send(fd[1-i], d->data + d->begin,
						 d->end - d->begin, MSG_DONTWAIT | MSG_NOSIGNAL);
				if (n > 0)
					d->begin += (size_t)n;
				else if (!n || (errno != EINTR && errno != EAGAIN && errno != EWOULDBLOCK)) {
					if (!n)
						errno = EPIPE;
					return native_proxy_error(n ? "send" : "send-zero", 1-i, errno);
				}
			}
			if (!d->eof && d->end < sizeof(d->data) && (ready[i].revents & (POLLIN | POLLHUP))) {
				ssize_t n = recv(fd[i], d->data + d->end,
						 sizeof(d->data) - d->end, MSG_DONTWAIT);
				if (n > 0)
					d->end += (size_t)n;
				else if (!n) {
					NATIVE_PROXY_TRACE("eof", i, 0);
					d->eof = 1;
				}
				else if (errno != EINTR && errno != EAGAIN && errno != EWOULDBLOCK)
					return native_proxy_error("recv", i, errno);
			}
		}
	}
}

#endif

// SPDX-License-Identifier: GPL-2.0-or-later
#ifndef MULTIKERNEL_NATIVE_STREAM_PROXY_H
#define MULTIKERNEL_NATIVE_STREAM_PROXY_H

#include <errno.h>
#include <stdint.h>
#include <time.h>
#include <fcntl.h>
#include <poll.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#ifndef NATIVE_PROXY_TRACE
#define NATIVE_PROXY_TRACE(stage, endpoint, detail) ((void)0)
#endif

/* Only the caller that selected Multikernel VSOCK may interpret its ring-full
 * ENOSPC as an unsent write. Other endpoints retain normal fatal-error policy. */
#ifndef NATIVE_PROXY_MK_ENDPOINT
#define NATIVE_PROXY_MK_ENDPOINT (-1)
#endif
#ifndef NATIVE_PROXY_QUEUE_TIMEOUT_MS
#define NATIVE_PROXY_QUEUE_TIMEOUT_MS 15000
#endif

static int native_proxy_now(int64_t *milliseconds)
{
	struct timespec now;
	if (clock_gettime(CLOCK_MONOTONIC, &now) < 0)
		return -1;
	*milliseconds = (int64_t)now.tv_sec * 1000 + now.tv_nsec / 1000000;
	return 0;
}

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
		int eof, shutdown, retry_delay;
		int64_t retry_after, retry_started;
	} direction[2] = {0};
	int fd[2] = {left, right};
	int queue_full_noted = 0;
	for (int i = 0; i < 2; i++) {
		int flags = fcntl(fd[i], F_GETFL);
		if (flags < 0 || fcntl(fd[i], F_SETFL, flags | O_NONBLOCK) < 0)
			return native_proxy_error("fcntl", i, errno);
	}
	for (;;) {
		struct pollfd ready[2] = {{.fd = left}, {.fd = right}};
		int completed = 0, timeout = -1;
		int write_allowed[2] = {1, 1};
		int64_t now = 0;
		if ((direction[0].retry_after || direction[1].retry_after) && native_proxy_now(&now) < 0)
			return native_proxy_error("clock", -1, errno);
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
			if (d->end > d->begin) {
				int allow_write = 1;
				if (d->retry_after) {
					int64_t budget = NATIVE_PROXY_QUEUE_TIMEOUT_MS - (now - d->retry_started);
					if (budget <= 0)
						return native_proxy_error("mk-queue-timeout", 1-i, ETIMEDOUT);
					if (timeout < 0 || budget < timeout) timeout = (int)budget;
					int64_t pause = d->retry_after - now;
					if (pause > 0) {
						allow_write = 0;
						write_allowed[i] = 0;
						if (pause < timeout) timeout = (int)pause;
					}
				}
				if (allow_write) ready[1-i].events |= POLLOUT;
			}
		}
		if (completed == 2) {
			NATIVE_PROXY_TRACE("complete", -1, 0);
			return 0;
		}
		/* Ignore a fully drained half so persistent HUP cannot cause a spin. */
		for (int i = 0; i < 2; i++)
			if (!ready[i].events)
				ready[i].fd = -1;
		int result = poll(ready, 2, timeout);
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
			if (d->end > d->begin && write_allowed[i] && (ready[1-i].revents & (POLLOUT | POLLHUP))) {
				ssize_t n = send(fd[1-i], d->data + d->begin,
						 d->end - d->begin, MSG_DONTWAIT | MSG_NOSIGNAL);
				if (n > 0) {
					d->begin += (size_t)n;
					d->retry_after = d->retry_started = 0;
					d->retry_delay = 0;
				} else if (n < 0 && errno == ENOSPC && 1-i == NATIVE_PROXY_MK_ENDPOINT) {
					if (native_proxy_now(&now) < 0)
						return native_proxy_error("clock", 1-i, errno);
					if (!d->retry_after) d->retry_started = now;
					d->retry_delay = d->retry_delay ? (d->retry_delay < 16 ? d->retry_delay * 2 : 16) : 1;
					d->retry_after = now + d->retry_delay;
					if (!queue_full_noted) {
						NATIVE_PROXY_TRACE("mk-queue-full", 1-i, ENOSPC);
						queue_full_noted = 1;
					}
				} else if (!n || (errno != EINTR && errno != EAGAIN && errno != EWOULDBLOCK)) {
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

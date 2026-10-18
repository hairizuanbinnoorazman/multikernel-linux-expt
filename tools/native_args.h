/* SPDX-License-Identifier: GPL-2.0-or-later */
#ifndef MULTIKERNEL_NATIVE_ARGS_H
#define MULTIKERNEL_NATIVE_ARGS_H

#include <stdint.h>
#include <stddef.h>

/* Reject signs, whitespace, suffixes and overflow before any host operation. */
static inline int native_decimal(const char *text, uint64_t minimum,
				 uint64_t maximum, uint64_t *result)
{
	uint64_t value = 0;
	if (!text || !*text || minimum > maximum)
		return 0;
	for (const unsigned char *cursor = (const unsigned char *)text; *cursor; cursor++) {
		if (*cursor < '0' || *cursor > '9')
			return 0;
		uint64_t digit = *cursor - '0';
		if (digit > maximum || value > (maximum - digit) / 10)
			return 0;
		value = value * 10 + digit;
	}
	if (value < minimum)
		return 0;
	*result = value;
	return 1;
}

static inline int native_identity(const char *text, size_t maximum)
{
	size_t length = 0;
	if (!text || !*text)
		return 0;
	for (const unsigned char *cursor = (const unsigned char *)text; *cursor; cursor++) {
		int alphanumeric = (*cursor >= 'a' && *cursor <= 'z') ||
			(*cursor >= 'A' && *cursor <= 'Z') || (*cursor >= '0' && *cursor <= '9');
		if ((!alphanumeric && (!length || (*cursor != '.' && *cursor != '_' && *cursor != '-'))) ||
		    ++length > maximum)
			return 0;
	}
	return 1;
}

#endif

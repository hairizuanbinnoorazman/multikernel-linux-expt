#include <stdio.h>
#include "../tools/native_args.h"

int main(void)
{
	const struct {
		const char *text;
		uint64_t minimum, maximum, expected;
		int valid;
	} cases[] = {
		{"0", 0, UINT32_MAX - 1, 0, 1},
		{"1", 1, UINT32_MAX - 1, 1, 1},
		{"4294967294", 0, UINT32_MAX - 1, UINT32_MAX - 1, 1},
		{"18446744073709551615", 0, UINT64_MAX, UINT64_MAX, 1},
		{"0004096", 4096, UINT64_MAX, 4096, 1},
		{"", 0, UINT64_MAX, 0, 0},
		{"-1", 0, UINT64_MAX, 0, 0},
		{"+1", 0, UINT64_MAX, 0, 0},
		{" 1", 0, UINT64_MAX, 0, 0},
		{"1 ", 0, UINT64_MAX, 0, 0},
		{"1x", 0, UINT64_MAX, 0, 0},
		{"0x10", 0, UINT64_MAX, 0, 0},
		{"0", 1, UINT32_MAX - 1, 0, 0},
		{"4294967295", 0, UINT32_MAX - 1, 0, 0},
		{"18446744073709551616", 0, UINT64_MAX, 0, 0},
		{"9", 0, 5, 0, 0},
		{"1", 2, 1, 0, 0},
	};
	for (size_t index = 0; index < sizeof(cases)/sizeof(cases[0]); index++) {
		uint64_t result = 123;
		int valid = native_decimal(cases[index].text, cases[index].minimum,
					   cases[index].maximum, &result);
		if (valid != cases[index].valid || result != (valid ? cases[index].expected : 123)) {
			fprintf(stderr, "decimal boundary case %zu failed\n", index);
			return 1;
		}
	}
	if (!native_identity("root-0123_abcd.1", 63) || native_identity("", 63) ||
	    native_identity("-identity", 63) || native_identity("identity\n", 63) ||
	    native_identity("identity/path", 63) || native_identity("abcdef", 5))
		return 1;
	puts("NATIVE_HELPER_ARGUMENT_BOUNDARIES_PASS");
	return 0;
}

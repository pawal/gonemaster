/* Loads the shared library and drives one offline run through the C ABI. */
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#include "libgonemaster_engine.h"

static int fail(const char *what, const char *got) {
	fprintf(stderr, "%s: %s\n", what, got);
	return 1;
}

int main(void) {
	char *s = gm_version_json();
	if (strstr(s, "\"engine_version\"") == NULL) return fail("version", s);
	gm_free_string(s);

	s = gm_list_tests_json();
	if (strstr(s, "\"basic01\"") == NULL) return fail("list", s);
	gm_free_string(s);

	s = gm_check_profile_json("{\"net\":1}");
	if (strstr(s, "unknown_property") == NULL || strstr(s, "\"effective\"") != NULL) return fail("check", s);
	gm_free_string(s);

	s = gm_check_profile_json("{\"net\":{\"ipv4\":false}}");
	if (strstr(s, "\"errors\":[]") == NULL || strstr(s, "\"effective\":{") == NULL) return fail("effective", s);
	gm_free_string(s);

	s = gm_translate_json("{\"language\":\"sv\",\"entries\":[{\"module\":\"System\",\"tag\":\"GLOBAL_VERSION\",\"args\":{\"version\":\"v1\"}}]}");
	if (strstr(s, "Anv\xc3\xa4nder version v1") == NULL) return fail("translate", s);
	gm_free_string(s);

	s = gm_normalize_name_json("Example.COM.");
	if (strstr(s, "\"name\":\"example.com\"") == NULL) return fail("normalize", s);
	gm_free_string(s);

	uint64_t h = gm_run_start("{\"domain\":\"example.com\",\"testcases\":[\"syntax01\"],\"ipv4\":false,\"ipv6\":false,\"include_timings\":true}");
	int entries = 0, timings = 0, done = 0;
	while (!done) {
		s = gm_run_next(h);
		if (strstr(s, "\"type\":\"entry\"") != NULL) entries++;
		else if (strstr(s, "\"type\":\"timings\"") != NULL) timings++;
		else if (strstr(s, "\"type\":\"done\"") != NULL) done = 1;
		else return fail("run", s);
		gm_free_string(s);
	}
	gm_run_free(h);
	if (entries == 0 || timings != 1) {
		fprintf(stderr, "entries=%d timings=%d\n", entries, timings);
		return 1;
	}

	s = gm_run_next(h);
	if (strstr(s, "unknown run handle") == NULL) return fail("freed handle", s);
	gm_free_string(s);

	puts("ok");
	return 0;
}

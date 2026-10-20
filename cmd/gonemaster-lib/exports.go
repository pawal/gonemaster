//go:build cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import "unsafe"

//export gm_version_json
func gm_version_json() *C.char { return C.CString(versionJSON()) }

//export gm_list_tests_json
func gm_list_tests_json() *C.char { return C.CString(listTestsJSON()) }

//export gm_check_profile_json
func gm_check_profile_json(profile *C.char) *C.char {
	return C.CString(checkProfileJSON(C.GoString(profile)))
}

//export gm_run_start
func gm_run_start(request *C.char) C.uint64_t { return C.uint64_t(runStart(C.GoString(request))) }

//export gm_run_next
func gm_run_next(handle C.uint64_t) *C.char { return C.CString(runNext(uint64(handle))) }

//export gm_run_cancel
func gm_run_cancel(handle C.uint64_t) { runCancel(uint64(handle)) }

//export gm_run_free
func gm_run_free(handle C.uint64_t) { runFree(uint64(handle)) }

//export gm_free_string
func gm_free_string(s *C.char) { C.free(unsafe.Pointer(s)) }

//export gm_translate_json
func gm_translate_json(request *C.char) *C.char {
	return C.CString(translateJSON(C.GoString(request)))
}

//export gm_normalize_name_json
func gm_normalize_name_json(name *C.char) *C.char {
	return C.CString(normalizeNameJSON(C.GoString(name)))
}

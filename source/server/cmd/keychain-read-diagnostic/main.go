//go:build darwin && cgo

// A one-shot, fixed-target diagnostic. Never prints native error descriptions,
// item attributes, paths, identities, or data. It never writes credentials.
package main

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static int noInteraction(void) { return SecKeychainSetUserInteractionAllowed(false); }
static int defaultStatus(void) {
 SecKeychainRef k = NULL;
 OSStatus s = SecKeychainCopyDefault(&k);
 if (k) CFRelease(k);
 return s;
}
static int searchStatus(int *count) {
 CFArrayRef a = NULL;
 OSStatus s = SecKeychainCopySearchList(&a);
 *count = a ? (int)CFArrayGetCount(a) : 0;
 if (a) CFRelease(a);
 return s;
}
// Only compares a default-keychain path with HOME; never exports the path.
static int defaultUnderHome(void) {
 SecKeychainRef k = NULL;
 if (SecKeychainCopyDefault(&k) != 0 || !k) return -1;
 char p[4096]; UInt32 n = sizeof(p);
 OSStatus s = SecKeychainGetPath(k, &n, p);
 CFRelease(k);
 const char *h = getenv("HOME");
 if (s || !h) return -1;
 size_t hn = strlen(h);
 return n > hn && !strncmp(p, h, hn) && p[hn] == '/';
}
static int query(int data, int *count) {
 CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
   &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
 CFDictionarySetValue(q, kSecAttrService, CFSTR("cercano"));
 CFDictionarySetValue(q, kSecAttrAccount, CFSTR("openai-responses"));
 CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
 CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
 CFDictionarySetValue(q, kSecReturnData, data ? kCFBooleanTrue : kCFBooleanFalse);
 if (!data) CFDictionarySetValue(q, kSecReturnRef, kCFBooleanTrue);
 CFTypeRef r = NULL;
 OSStatus s = SecItemCopyMatching(q, &r);
 *count = r ? 1 : 0;
 // Never inspect or copy any returned item attributes/data.
 if (r) CFRelease(r);
 CFRelease(q);
 return s;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"

	native "github.com/99designs/go-keychain"
	"github.com/99designs/keyring"
)

func classification(s int) string {
	if s == 0 {
		return "success"
	}
	if s == -25300 {
		return "missing"
	}
	return "error"
}

func main() {
	// Keep the process-wide native no-interaction setting and all calls on the
	// same OS thread as an additional precaution. No helper may authorize UI.
	runtime.LockOSThread()
	keyring.Debug = false
	s := int(C.noInteraction())
	fmt.Printf("no_interaction classification=%s osstatus=%d\n", classification(s), s)
	if s != 0 {
		return
	}
	s = int(C.defaultStatus())
	fmt.Printf("default_keychain classification=%s osstatus=%d under_home=%d\n", classification(s), s, int(C.defaultUnderHome()))
	var count C.int
	s = int(C.searchStatus(&count))
	fmt.Printf("search_list classification=%s osstatus=%d count=%d\n", classification(s), s, int(count))
	kr, err := keyring.Open(keyring.Config{ServiceName: "cercano", KeychainTrustApplication: true})
	if err != nil {
		fmt.Println("backend classification=error")
		return
	}
	backend := "other"
	if reflect.TypeOf(kr).String() == "*keyring.keychain" {
		backend = "keychain"
	}
	fmt.Printf("backend=%s\n", backend)
	if backend != "keychain" {
		return
	}
	// Actual replacement adapter metadata query: attributes stay inside the
	// process and are never emitted; no data is requested.
	md, err := kr.GetMetadata("openai-responses")
	md.Item = nil
	_ = md
	if err == nil {
		fmt.Println("adapter_metadata classification=success osstatus=0 count=1")
	} else if errors.Is(err, keyring.ErrKeyNotFound) {
		fmt.Println("adapter_metadata classification=missing count=0")
	} else if e, ok := err.(native.Error); ok {
		fmt.Printf("adapter_metadata classification=error osstatus=%d count=0\n", int(e))
	} else {
		fmt.Println("adapter_metadata classification=error")
	}
	for _, data := range []int{0, 1} {
		count = 0
		s = int(C.query(C.int(data), &count))
		fmt.Printf("native_query data_requested=%d classification=%s osstatus=%d count=%d\n", data, classification(s), s, int(count))
	}
}

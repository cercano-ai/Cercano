//go:build realcloudacceptance && darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>

static int readAcceptanceCredential(int interactive) {
 OSStatus status = SecKeychainSetUserInteractionAllowed(interactive ? true : false);
 if (status != errSecSuccess) return status;
 CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
   &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 if (!q) return errSecAllocate;
 CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
 CFDictionarySetValue(q, kSecAttrService, CFSTR("cercano"));
 CFDictionarySetValue(q, kSecAttrAccount, CFSTR("openai-responses"));
 CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
 CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
 CFTypeRef result = NULL;
 status = SecItemCopyMatching(q, &result);
 // Discard immediately: never inspect/copy data into Go, format, or persist it.
 if (result) CFRelease(result);
 CFRelease(q);
 return status;
}
*/
import "C"

import (
	"runtime"
)

func queryAcceptanceCredential(interactive bool) int {
	return runAcceptanceCredential(interactive, func(allow bool) int {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		flag := C.int(0)
		if allow {
			flag = 1
		}
		return int(C.readAcceptanceCredential(flag))
	})
}

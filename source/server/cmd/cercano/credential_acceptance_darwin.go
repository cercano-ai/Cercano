//go:build realcloudacceptance && darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>

static int readAcceptanceCredential(void) {
 OSStatus status = SecKeychainSetUserInteractionAllowed(false);
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
	"os"
	"runtime"
)

func queryAcceptanceCredential() int {
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") == "1" {
		return -4
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return int(C.readAcceptanceCredential())
}

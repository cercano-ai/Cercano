package proto

import "testing"
import "google.golang.org/protobuf/reflect/protoreflect"

func TestExperimentDisclosureContract(t *testing.T) {
	for _, m := range []interface{ ProtoReflect() protoreflect.Message }{&PermissionRequest{}, &PermissionRequired{}} {
		if m.ProtoReflect().Descriptor().Fields().ByName("disclosure") == nil {
			t.Errorf("%s drops experiment disclosure", m.ProtoReflect().Descriptor().Name())
		}
	}
}

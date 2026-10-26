package proto

import "testing"

func TestLifecycleControlsAdditiveMethods(t *testing.T) {
	service := File_agent_proto.Services().ByName("Agent")
	for _, name := range []string{"LifecycleAcquire", "LifecycleRelease", "LifecyclePrepare", "LifecycleValidateReceipt", "LifecycleRelinquish", "LifecycleClaim", "LifecycleList", "LifecycleRetire"} {
		found := false
		for i := 0; i < service.Methods().Len(); i++ {
			m := service.Methods().Get(i)
			if string(m.Name()) == name {
				found = true
				if string(m.Input().Name()) != "LifecycleControlRequest" || string(m.Output().Name()) != "LifecycleControlResponse" {
					t.Fatal(name)
				}
			}
		}
		if !found {
			t.Fatal("missing control", name)
		}
	}
}

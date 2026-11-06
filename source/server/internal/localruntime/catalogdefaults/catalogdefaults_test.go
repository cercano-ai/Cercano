package catalogdefaults

import (
	"reflect"
	"testing"

	"cercano/source/server/internal/localruntime/llamaserver"
	"cercano/source/server/internal/localruntime/mistralrs"
)

func TestForRuntimeCompatibility(t *testing.T) {
	const ram = 8 << 30
	for _, runtime := range []string{"ollama", "OLLAMA", "unknown", ""} {
		t.Run(runtime, func(t *testing.T) {
			got := ForRuntime(runtime, ram)
			t.Logf("runtime=%q RAM=%d defaults=%v", runtime, ram, got)
			if len(got) != 0 {
				t.Fatalf("unsupported runtime must not inherit native catalog defaults: %v", got)
			}
		})
	}
	for _, runtime := range []string{"llama_server", "LLAMA_SERVER"} {
		if got, want := ForRuntime(runtime, ram), llamaserver.RecommendedOpenModels(ram); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s defaults=%v, want %v", runtime, got, want)
		}
	}
	want := mistralrs.RecommendedOpenModels(ram)
	want["embedding"] = llamaserver.RecommendedOpenModels(ram)["embedding"]
	if got := ForRuntime("mistralrs", ram); !reflect.DeepEqual(got, want) {
		t.Fatalf("mistralrs defaults=%v, want %v", got, want)
	}
}

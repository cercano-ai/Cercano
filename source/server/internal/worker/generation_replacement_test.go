package worker

import (
	"os"
	"path/filepath"
	"testing"
)

// The pinned path alone is insufficient: replacing a generation member after
// host startup must not silently redirect a later worker spawn.
func TestDevHostWorkerGenerationReplacementRejected(t *testing.T) {
	for _, member := range []string{"cercano", "manifest.json"} {
		t.Run(member, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			generation := filepath.Join(root, "generation")
			if err := os.Mkdir(generation, 0700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string]string{"cercano": "fixture A", "manifest.json": `{"build":"A"}`} {
				if err := os.WriteFile(filepath.Join(generation, name), []byte(data), 0700); err != nil {
					t.Fatal(err)
				}
			}
			policy, err := NewSpawnPolicy(SpawnOptions{GenerationDir: generation, ManifestPath: filepath.Join(generation, "manifest.json"), HostInstance: "fixture", ResourceRoot: root})
			if err != nil {
				t.Fatal(err)
			}
			replacement := "fixture B"
			if member == "manifest.json" {
				replacement = `{"build":"B"}`
			}
			if err := os.WriteFile(filepath.Join(generation, member), []byte(replacement), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := policy.workerBinary(); err == nil {
				t.Fatal("changed generation accepted")
			}
		})
	}
}

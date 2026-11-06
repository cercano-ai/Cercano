package main

// Generation artifacts are immutable build identity. Instance artifacts are
// immutable host identity; the per-generation pointer is discovery only.
import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cercano/source/server/internal/brewrestart"
	pb "cercano/source/server/pkg/proto"
	"golang.org/x/term"
)

func developmentInstanceMatches(base string, intent map[string]string) bool {
	generation, host := intent["generation"], intent["host"]
	if generation == "" || host == "" {
		return false
	}
	legacy := fmt.Sprintf("%x", sha256.Sum256([]byte(generation)))
	if filepath.Base(base) == legacy {
		return true
	}
	// New instances use the already unique lifecycle host identity. Do not allow
	// a path supplied by a record or pointer to escape the private hosts directory.
	return len(host) == 32 && strings.Trim(host, "0123456789abcdef") == "" && filepath.Base(base) == legacy+"-"+host
}

func developmentInstanceBase(generationBase string) (string, error) {
	var pointer struct{ Base string }
	err := launchRead(generationBase+".instance", &pointer)
	if os.IsNotExist(err) {
		return generationBase, nil
	}
	if err != nil {
		return "", err
	}
	if filepath.Base(pointer.Base) != pointer.Base || !strings.HasPrefix(pointer.Base, filepath.Base(generationBase)+"-") {
		return "", fmt.Errorf("invalid generation instance pointer")
	}
	base := filepath.Join(filepath.Dir(generationBase), pointer.Base)
	var intent map[string]string
	if err = launchRead(base+".starting", &intent); err != nil {
		return "", err
	}
	if filepath.Dir(base) != filepath.Dir(generationBase) || !strings.HasPrefix(pointer.Base, filepath.Base(generationBase)+"-") || !developmentInstanceMatches(base, intent) {
		return "", fmt.Errorf("invalid generation instance pointer")
	}
	return base, nil
}

func developmentNewInstance(generationBase, generation, authority string) (string, string, error) {
	host, err := launchID()
	if err != nil {
		return "", "", err
	}
	base := generationBase + "-" + host
	if err = launchExclusive(base+".starting", map[string]string{"host": host, "generation": generation, "config_authority": authority}); err != nil {
		return "", "", err
	}
	// Publish discovery before spawning: an interrupted attempt is never invisible.
	f, err := os.CreateTemp(filepath.Dir(base), ".instance-*")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(struct{ Base string }{filepath.Base(base)})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return "", "", err
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	if err = os.Rename(f.Name(), generationBase+".instance"); err != nil {
		return "", "", err
	}
	dir, err := os.Open(filepath.Dir(base))
	if err != nil {
		return "", "", err
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return "", "", err
	}
	return base, host, nil
}

func developmentConfirm(message string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	fmt.Fprint(os.Stderr, message+" [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return err == nil && (strings.TrimRight(line, "\r\n") == "y" || strings.TrimRight(line, "\r\n") == "Y")
}

// Called only inside both launch exclusions. Absence is kernel-verified; neither
// RPC failure nor a missing record proves death. Every branch preserves records.
func developmentReplaceBlocked(ctx context.Context, base, generation, authority string, s launchSelection, cause error, confirm func(string) bool) error {
	return developmentReplaceBlockedWith(ctx, base, generation, authority, s, cause, confirm, developmentRecoveryOps{brewrestart.VerifyHostAbsent, brewrestart.PrepareLegacyHost, brewrestart.StopLegacyHost})
}

// Invocation-local dependencies keep consent/process-control tests entirely isolated.
type developmentRecoveryOps struct {
	absent  func(context.Context, string) error
	prepare func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error)
	stop    func(context.Context, string, netip.AddrPort, brewrestart.Identity, func() error) error
}

func developmentReplaceBlockedWith(ctx context.Context, base, generation, authority string, s launchSelection, cause error, confirm func(string) bool, ops developmentRecoveryOps) error {
	var intent map[string]string
	err := launchRead(base+".starting", &intent)
	if err == nil && (!developmentInstanceMatches(base, intent) || intent["generation"] != generation || intent["config_authority"] != authority || (s.Identity != nil && s.Identity.GetHost() != intent["host"])) {
		return fmt.Errorf("blocked host start intent mismatch")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	host := s.Identity.GetHost()
	if err == nil {
		host = intent["host"]
	}
	if host != "" && ops.absent(ctx, host) == nil {
		return nil
	}
	// An authenticated record alone is not a safe process-control target. The
	// existing single-host helper also verifies executable, PID start and listener.
	if s.Identity != nil {
		endpoint, parseErr := netip.ParseAddrPort(s.Endpoint)
		if parseErr == nil {
			id, inspectErr := ops.prepare(ctx, host, endpoint)
			if inspectErr == nil && id.Executable == filepath.Join(generation, "cercano") {
				fmt.Fprintf(os.Stderr, "[cercano] Blocking host %s at %s; record %s.json. Readiness/admission failed: %v\n", host, s.Endpoint, base, cause)
				ready, cancel := context.WithTimeout(ctx, 5*time.Second)
				controls, controlErr := launchControls(ready, s)
				if controlErr == nil {
					inventory, listErr := controls.LifecycleList(ready, &pb.LifecycleControlRequest{Target: s.Identity, Management: true})
					if listErr == nil {
						fmt.Fprintf(os.Stderr, "[cercano] Host inventory: %v\n", inventory)
					} else {
						fmt.Fprintf(os.Stderr, "[cercano] Session/work inventory unavailable: %v\n", listErr)
					}
					controls.Close()
				} else {
					fmt.Fprintf(os.Stderr, "[cercano] Session/work inventory unavailable: %v\n", controlErr)
				}
				cancel()
				if !confirm("Stop this verified host and relaunch? Its sessions will disconnect and in-flight work may be lost. No other host will be stopped.") {
					return fmt.Errorf("stop/relaunch declined; refusing replacement: %w", cause)
				}
				// Recheck immutable source bytes while still holding the launch lock. The
				// stop helper rechecks kernel identity and waits for confirmed process exit.
				stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
				defer stopCancel()
				return ops.stop(stopCtx, host, endpoint, id, func() error {
					var again launchSelection
					if err := launchRead(base+".json", &again); err != nil {
						return err
					}
					a, _ := json.Marshal(s)
					b, _ := json.Marshal(again)
					if string(a) != string(b) {
						return fmt.Errorf("approved blocking host record changed")
					}
					return nil
				})
			}
		}
	}
	if !confirm(fmt.Sprintf("Host %s liveness cannot be safely established (%v). Force launch a separate host of this same selected build? The old host and records will be left untouched; old conversations remain owned there and are not recovered. No database migration is authorized.", host, cause)) {
		return fmt.Errorf("force launch declined; refusing replacement: %w", cause)
	}
	return nil
}

func developmentInstanceLock(base string) (*os.File, error) {
	if err := devHostSupported(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR|devOpenNoFollow, 0600)
	if err != nil {
		return nil, err
	}
	if err = launchPrivateFile(base + ".lock"); err == nil {
		err = devFlock(f, devLockExclusive)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

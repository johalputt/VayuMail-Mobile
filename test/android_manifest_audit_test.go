package test

// android_manifest_audit_test.go — the backup control has to exist in the
// build, not only in the prose describing the build.
//
// The finding, in the attacker's voice: I do not need root and I do not need
// the passphrase. `adb backup`, or a cloud Auto Backup restore onto a device I
// control, hands me the app-private directory — vayumail.db with every message
// body, plus the sealed keystore and its key file. Android gives me that by
// default, and the only thing that would stop me is one manifest attribute.
//
// This was documented as fixed. CHANGELOG 2.2.13 carried "Android backup of the
// app-private data is disabled (audit L12)" under Security, and both rule files
// under platform/android/ stated in the present tense that the app sets
// allowBackup="false". None of it was in a shipped APK: gogio compiles the
// manifest into itself, its <application> tag has no allowBackup, and the two
// rule files were referenced by nothing in the build.
//
// The gap between a security claim and a security control is the thing this
// file exists to close. A claim nobody can fail is not a control.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pipelineRunsHardenedGogio reports whether release.yml actually RUNS the
// hardening script, ignoring comments.
//
// Read from executable lines only, because the first version of this helper
// matched the YAML comment that explains why the script exists — so removing
// the real step left the check green. A gate that its own documentation can
// satisfy is not a gate.
func pipelineRunsHardenedGogio(wf string) bool {
	for _, line := range strings.Split(wf, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if strings.Contains(l, "scripts/gogio-hardened.sh") {
			return true
		}
	}
	return false
}

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{".."}, rel...)...))
	if err != nil {
		t.Fatalf("read %v: %v", rel, err)
	}
	return string(b)
}

// The release workflow must build the hardened gogio. A bare
// `go install gioui.org/cmd/gogio` produces a tool whose manifest leaves backup
// on, so its presence here is the regression, not a style preference.
func TestTheReleasePipelineBuildsTheHardenedGogio(t *testing.T) {
	wf := readRepoFile(t, ".github", "workflows", "release.yml")

	if !pipelineRunsHardenedGogio(wf) {
		t.Error("release.yml does not run scripts/gogio-hardened.sh. Without it gogio emits an " +
			"<application> tag with no android:allowBackup, Android defaults it to true, and " +
			"vayumail.db plus the sealed keystore are eligible for adb and cloud backup.")
	}

	// Look for an unhardened install that would silently win by running later.
	for _, line := range strings.Split(wf, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") {
			continue
		}
		if strings.Contains(l, "go install") && strings.Contains(l, "cmd/gogio") {
			t.Errorf("release.yml still installs gogio directly (%q). That overwrites the hardened "+
				"build on $PATH and the APK goes out with backup enabled.", l)
		}
	}
}

// And the script has to actually assert its patch applied. A patch that
// silently no-ops when gogio's template changes is the worst outcome here: the
// pipeline stays green and the control quietly stops existing.
func TestTheGogioPatchFailsLoudlyIfItStopsApplying(t *testing.T) {
	sh := readRepoFile(t, "scripts", "gogio-hardened.sh")

	if !strings.Contains(sh, `android:allowBackup="false"`) {
		t.Fatal("scripts/gogio-hardened.sh no longer injects allowBackup=\"false\"")
	}
	if !strings.Contains(sh, "n != 1") {
		t.Error("the patch does not assert that it matched exactly one <application> line. " +
			"Without that assertion a gogio upgrade that reworded the template would leave the " +
			"patch matching nothing, the build green, and backup enabled.")
	}
	if !strings.Contains(sh, "GOGIO_VERSION") {
		t.Error("gogio is not pinned. The patch is tied to a known template, and a release that " +
			"resolves its build tool at @latest cannot be reasoned about afterwards.")
	}
}

// The claim check. Nothing in the tree may state that the app sets
// allowBackup="false" unless the pipeline is what sets it.
//
// This is deliberately the same shape as the defect it caught: the rule files
// asserted the control in the present tense while nothing implemented it, and
// no test could tell the difference between that and the truth.
func TestNothingClaimsBackupIsDisabledUnlessThePipelineDisablesIt(t *testing.T) {
	wf := readRepoFile(t, ".github", "workflows", "release.yml")
	hardened := pipelineRunsHardenedGogio(wf)

	var claims []string
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".xml", ".md":
		default:
			return nil
		}
		// This file names the string it forbids while explaining it, and a
		// gate that matches its own text reports a violation in the sentence
		// describing the violation.
		if strings.HasSuffix(path, "android_manifest_audit_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for _, line := range strings.Split(string(b), "\n") {
			low := strings.ToLower(line)
			if !strings.Contains(low, "allowbackup") {
				continue
			}
			// "The app sets …" / "the manifest sets …" — a statement of fact.
			if strings.Contains(low, " sets ") || strings.Contains(low, "is disabled") {
				claims = append(claims, path+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if !hardened && len(claims) > 0 {
		t.Errorf("the build does not disable Android backup, but %d file(s) say it does:\n  %s\n"+
			"Either wire the manifest patch into release.yml or stop asserting a control that "+
			"does not ship. A security claim a reader cannot check is worse than an admitted gap.",
			len(claims), strings.Join(claims, "\n  "))
	}
}

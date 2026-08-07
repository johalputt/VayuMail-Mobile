package screens

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The message on screen must carry the reason, not a theory about it.
//
// The first version of this branch told the user to check their screen lock,
// on the assumption that a failed credential write meant the platform's
// hardware keystore had declined. It does not: the store is a file-based
// sealed keystore, and its errors name the step that failed — master key
// corrupt, read, write, sync, key dir, nonce. Matching the error and replacing
// it with advice threw away the only line that says which.
//
// That is the same defect the surrounding fix exists to remove. A swallowed
// reason made the original failure look like a hang; swallowing it again would
// make it look like a screen-lock problem, which is no better and sends the
// operator somewhere there is nothing to find.
func TestAddAccountFailureKeepsTheKeystoreReason(t *testing.T) {
	for _, reason := range []string{
		"keystore: master key file corrupt",
		"keystore: write master key: no space left on device",
		"keystore: create key dir: permission denied",
	} {
		got := addAccountFailure(fmt.Errorf("syncmanager: store credential: %w", errors.New(reason)))

		if !strings.Contains(got, reason) {
			t.Errorf("the keystore said %q and the screen shows:\n\n%s\n\n"+
				"Without that sentence nobody can tell which step failed, and the next person "+
				"debugging this is back to guessing — which is how the screen-lock advice got here.",
				reason, got)
		}
		if strings.Contains(strings.ToLower(got), "screen lock") {
			t.Errorf("the message still advises a screen lock for %q.\n\n"+
				"This is a file-based sealed store; a screen lock has no bearing on it, so the "+
				"advice sends the operator to look somewhere there is nothing to find.", reason)
		}
	}
}

// The control: causes that genuinely are not the keystore keep their own plain
// wording, or "always print the raw error" would satisfy the test above while
// showing Go prose for a timeout.
func TestAddAccountFailureStillExplainsTheOtherCauses(t *testing.T) {
	if got := addAccountFailure(context.DeadlineExceeded); !strings.Contains(got, "did not finish") {
		t.Errorf("a timeout reads as %q, which does not tell the user to try again", got)
	}
	if got := addAccountFailure(errors.New("syncmanager: command queue full, try again")); !strings.Contains(got, "busy") {
		t.Errorf("a full queue reads as %q", got)
	}
}

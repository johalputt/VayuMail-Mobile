package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A mailbox at its device ceiling is not a network problem, and the app used to
// say it was. Every non-401, non-404 status collapsed into one generic error
// that the setup screen renders as "Couldn't register this device — check your
// connection and try again."
//
// So somebody whose device list was full was told to test their wifi, retried,
// got the same sentence, and had no way to reach the real remedy — which only
// an operator can perform, in the web console. Retrying is the one thing that
// cannot work, and it is exactly what the message asked for.
func TestRegisterDeviceReportsAFullDeviceList(t *testing.T) {
	const reason = "Device limit reached for this mailbox — remove an old device in the console first"

	body, _ := json.Marshal(map[string]string{"error": "device-limit", "message": reason})
	err := classifyDeviceStatus(&http.Response{
		StatusCode: http.StatusConflict,
		Body:       io.NopCloser(bytes.NewReader(body)),
	})

	if err == nil {
		t.Fatal("a 409 was treated as success")
	}
	if !errors.Is(err, ErrDeviceLimit) {
		t.Errorf("err = %v, want ErrDeviceLimit.\n\n"+
			"Folded into the generic error, the screen tells the user to check their connection — "+
			"and retrying a full device list can never clear it.", err)
	}
	if errors.Is(err, ErrDevice) && !errors.Is(err, ErrDeviceLimit) {
		t.Error("a full list must be distinguishable from a transport failure")
	}
	if !strings.Contains(err.Error(), "remove an old device") {
		t.Errorf("the server's own explanation did not survive: %v.\n\n"+
			"The remedy lives in that sentence; dropping it leaves a status code.", err)
	}
}

// The control: an ordinary transport-shaped failure must still be the generic
// error, or "distinguish the limit" would be satisfied by calling everything a
// limit.
func TestRegisterDeviceStillReportsOtherFailuresGenerically(t *testing.T) {
	err := classifyDeviceStatus(&http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	})

	if err == nil {
		t.Fatal("a 502 was treated as success")
	}
	if errors.Is(err, ErrDeviceLimit) {
		t.Errorf("a 502 was reported as a full device list: %v", err)
	}
	if !errors.Is(err, ErrDevice) {
		t.Errorf("err = %v, want ErrDevice", err)
	}
}

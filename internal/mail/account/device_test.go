package account

// device_test.go — RegisterDevice/DeviceStatus must POST to the
// account's own HTTPS host, refuse non-public domains before any request
// (SSRF), map 404/405 and non-endpoint bodies to ErrNoDeviceEndpoint so
// callers fall back on older VayuPress servers, and map 401 to
// ErrDeviceCredentials so a wrong password is reported, not deferred to
// IMAP. The stub RoundTripper stands in for the server (no network).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestRegisterDevicePending(t *testing.T) {
	var gotURL, gotMethod string
	var gotBody map[string]string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		return jsonResp(http.StatusOK,
			`{"device_id":"dev-1","device_password":"dp-secret","status":"pending"}`), nil
	})}

	grant, err := RegisterDevice(context.Background(), client,
		"u@example.com", "s3cret", "VayuMail on Linux", "linux")
	if err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if grant.DeviceID != "dev-1" || grant.DevicePassword != "dp-secret" || grant.Status != DeviceStatusPending {
		t.Errorf("grant = %+v", grant)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotURL != "https://example.com/api/v1/members/vayumail-device-register" {
		t.Errorf("url = %s", gotURL)
	}
	want := map[string]string{
		"email": "u@example.com", "password": "s3cret",
		"device_name": "VayuMail on Linux", "platform": "linux",
	}
	for k, v := range want {
		if gotBody[k] != v {
			t.Errorf("body[%s] = %q, want %q", k, gotBody[k], v)
		}
	}
}

func TestRegisterDeviceApproved(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK,
			`{"device_id":"dev-2","device_password":"dp2","status":"approved"}`), nil
	})}
	grant, err := RegisterDevice(context.Background(), client,
		"u@example.com", "p", "VayuMail on Android", "android")
	if err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if grant.Status != DeviceStatusApproved || grant.DevicePassword != "dp2" {
		t.Errorf("grant = %+v", grant)
	}
}

func TestRegisterDeviceNoEndpoint(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResp(code, `{"error":"no route"}`), nil
		})}
		_, err := RegisterDevice(context.Background(), client, "u@example.com", "p", "n", "linux")
		if !errors.Is(err, ErrNoDeviceEndpoint) {
			t.Errorf("status %d: err = %v, want ErrNoDeviceEndpoint", code, err)
		}
	}
}

func TestRegisterDeviceBadCredentials(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusUnauthorized, `{"error":"invalid-credentials"}`), nil
	})}
	_, err := RegisterDevice(context.Background(), client, "u@example.com", "wrong", "n", "linux")
	if !errors.Is(err, ErrDeviceCredentials) {
		t.Fatalf("err = %v, want ErrDeviceCredentials", err)
	}
	if errors.Is(err, ErrNoDeviceEndpoint) {
		t.Fatal("401 must not read as endpoint-absent — that would fall back to a password the server will reject")
	}
}

func TestRegisterDeviceRejectsLoopbackDomain(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResp(http.StatusOK, `{}`), nil
	})}
	for _, addr := range []string{"u@localhost", "u@127.0.0.1", "u@internal", "u@example.com:9999"} {
		if _, err := RegisterDevice(context.Background(), client, addr, "p", "n", "linux"); !errors.Is(err, ErrDevice) {
			t.Errorf("%s: err = %v, want ErrDevice", addr, err)
		}
	}
	if called {
		t.Fatal("transport was called for a rejected domain (SSRF guard bypassed)")
	}
}

func TestRegisterDeviceGarbageBody(t *testing.T) {
	// A 200 that is not the endpoint's JSON (an HTML front page behind a
	// misrouting proxy) must read as endpoint-absent so onboarding falls
	// back instead of dead-ending.
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `<!doctype html><html>welcome</html>`), nil
	})}
	if _, err := RegisterDevice(context.Background(), client, "u@example.com", "p", "n", "linux"); !errors.Is(err, ErrNoDeviceEndpoint) {
		t.Fatalf("err = %v, want ErrNoDeviceEndpoint", err)
	}
	// Valid JSON that lacks the grant fields is equally not the endpoint.
	client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"hello":"world"}`), nil
	})}
	if _, err := RegisterDevice(context.Background(), client, "u@example.com", "p", "n", "linux"); !errors.Is(err, ErrNoDeviceEndpoint) {
		t.Fatalf("err = %v, want ErrNoDeviceEndpoint", err)
	}
}

func TestDeviceStatus(t *testing.T) {
	var gotURL string
	var gotBody map[string]string
	for _, want := range []string{DeviceStatusPending, DeviceStatusApproved, DeviceStatusBlocked} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			return jsonResp(http.StatusOK, `{"status":`+jsonString(want)+`}`), nil
		})}
		got, err := DeviceStatus(context.Background(), client, "u@example.com", "dev-1", "dp")
		if err != nil || got != want {
			t.Errorf("DeviceStatus = %q, %v; want %q", got, err, want)
		}
	}
	if gotURL != "https://example.com/api/v1/members/vayumail-device-status" {
		t.Errorf("url = %s", gotURL)
	}
	if gotBody["email"] != "u@example.com" || gotBody["device_id"] != "dev-1" || gotBody["device_password"] != "dp" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestDeviceStatusUnknownValue(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"status":"weird"}`), nil
	})}
	if _, err := DeviceStatus(context.Background(), client, "u@example.com", "d", "p"); !errors.Is(err, ErrDevice) {
		t.Fatalf("err = %v, want ErrDevice", err)
	}
}

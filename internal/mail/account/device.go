package account

// device.go — register this install as a named device with the account's
// VayuPress server and poll its approval status (device-approval
// onboarding, ADR-0011; the server half is VayuPress ADR-0129). When the
// server enforces device approval, IMAP rejects the raw mailbox password
// and any pending or blocked device password — only an approved device's
// password syncs mail — so onboarding must obtain a grant and wait for
// approval. Transport discipline mirrors privkey.go: HTTPS only, the
// SSRF domain guard, and refused redirects, so the mailbox credential
// and the granted device password never traverse an unverified hop.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrNoDeviceEndpoint marks a server without the device-approval
// endpoints — an older VayuPress answering 404/405, or a front page /
// proxy serving something that is not the endpoint's JSON. Callers fall
// back to the plain-password flow, exactly the pre-approval behavior.
var ErrNoDeviceEndpoint = errors.New("account: no device-approval endpoint")

// ErrDeviceCredentials is returned when the server refused the mailbox
// credentials during registration (HTTP 401): the endpoint exists, the
// typed password is wrong, and falling back would only defer the failure
// to IMAP.
var ErrDeviceCredentials = errors.New("account: server rejected the mailbox credentials")

// ErrDevice is the generic device-request failure (bad domain, network,
// unexpected server status). The endpoint may exist, so callers should
// surface it rather than silently fall back.
var ErrDevice = errors.New("account: device request failed")

// maxDeviceRespBytes caps the response — the grant JSON is tiny.
const maxDeviceRespBytes = 64 << 10

// Device status values returned by the VayuPress endpoints.
const (
	DeviceStatusPending  = "pending"
	DeviceStatusApproved = "approved"
	DeviceStatusBlocked  = "blocked"
)

// DeviceGrant is the registration result: a device identity plus the
// per-device password that (once the device is approved) authenticates
// IMAP/SMTP. The password is a credential — keystore only, never SQLite
// (Rule 6).
type DeviceGrant struct {
	DeviceID       string
	DevicePassword string
	Status         string
}

// deviceRegisterResponse is the register endpoint's JSON shape.
type deviceRegisterResponse struct {
	DeviceID       string `json:"device_id"`
	DevicePassword string `json:"device_password"`
	Status         string `json:"status"`
}

// deviceStatusResponse is the status endpoint's JSON shape.
type deviceStatusResponse struct {
	Status string `json:"status"`
}

// RegisterDevice registers this install with email's VayuPress server,
// authenticating with the mailbox password, and returns the granted
// device identity. Status is "approved" (usable immediately) or
// "pending" (poll DeviceStatus until a human approves the device in the
// web console). ErrNoDeviceEndpoint means the server predates device
// approval and the caller must use the plain-password flow.
func RegisterDevice(ctx context.Context, client *http.Client, email, password, deviceName, platform string) (DeviceGrant, error) {
	var out deviceRegisterResponse
	err := postDeviceJSON(ctx, client, domainOf(email), "vayumail-device-register",
		map[string]string{
			"email":       email,
			"password":    password,
			"device_name": deviceName,
			"platform":    platform,
		}, &out)
	if err != nil {
		return DeviceGrant{}, err
	}
	if out.DeviceID == "" || out.DevicePassword == "" ||
		(out.Status != DeviceStatusPending && out.Status != DeviceStatusApproved) {
		// Decodable JSON that is not this endpoint's contract (e.g. a
		// generic JSON error page): treat the endpoint as absent so the
		// caller falls back safely.
		return DeviceGrant{}, fmt.Errorf("%w: unexpected response shape", ErrNoDeviceEndpoint)
	}
	return DeviceGrant(out), nil
}

// DeviceStatus polls the approval state of a previously registered
// device, authenticating with the device's own credential. It returns
// one of DeviceStatusPending, DeviceStatusApproved or DeviceStatusBlocked.
func DeviceStatus(ctx context.Context, client *http.Client, email, deviceID, devicePassword string) (string, error) {
	var out deviceStatusResponse
	err := postDeviceJSON(ctx, client, domainOf(email), "vayumail-device-status",
		map[string]string{
			"email":           email,
			"device_id":       deviceID,
			"device_password": devicePassword,
		}, &out)
	if err != nil {
		return "", err
	}
	switch out.Status {
	case DeviceStatusPending, DeviceStatusApproved, DeviceStatusBlocked:
		return out.Status, nil
	}
	return "", fmt.Errorf("%w: unknown status %q", ErrDevice, out.Status)
}

// postDeviceJSON POSTs payload to the domain's members API over HTTPS
// and decodes the JSON reply into out. Same discipline as
// FetchPrivateKey: the SSRF guard vets the domain before any request,
// and redirects are refused so credentials cannot be replayed to a
// different host.
func postDeviceJSON(ctx context.Context, client *http.Client, domain, endpoint string, payload map[string]string, out any) error {
	if !publicMailDomain(domain) {
		return fmt.Errorf("%w: bad domain", ErrDevice)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := "https://" + domain + "/api/v1/members/" + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDevice, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		// Older VayuPress without the endpoint.
		return fmt.Errorf("%w: server returned %d", ErrNoDeviceEndpoint, resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: server returned 401", ErrDeviceCredentials)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: server returned %d", ErrDevice, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxDeviceRespBytes)).Decode(out); err != nil {
		// A 200 that is not JSON is a storefront or proxy page, not the
		// endpoint: treat it as absent so old setups keep working.
		return fmt.Errorf("%w: %v", ErrNoDeviceEndpoint, err)
	}
	return nil
}

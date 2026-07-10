# platform/ios

Build output and property-list overrides for the iOS target.

```sh
make ios    # runs gogio -target ios ./cmd/vayumail (requires Xcode)
```

No Info.plist usage-description keys are required — QR scanning was
retired at v2.0.0 (ADR-0009), so the app touches no camera, microphone,
location, or contacts API.

Entitlements: Keychain access group for credential storage (ADR-0004).
No push entitlement at v0.1.0 — APNs is deferred (COMPLIANCE-TRACKER.md:
"iOS APNs", PENDING); iOS syncs while the app is foregrounded.

Pending platform work: Keychain bridge
(`internal/crypto.PlatformBridge`), APNs registration (Phase 5).

# platform/ios

Build output and property-list overrides for the iOS target.

```sh
make ios    # runs gogio -target ios ./cmd/vayumail (requires Xcode)
```

Info.plist additions on top of the gogio output:

```xml
<key>NSCameraUsageDescription</key>
<string>VayuMail uses the camera only to scan your mail server's setup QR code.</string>
```

Entitlements: Keychain access group for credential storage (ADR-0004).
No push entitlement at v0.1.0 — APNs is deferred (COMPLIANCE-TRACKER.md:
"iOS APNs", PENDING); iOS syncs while the app is foregrounded.

Pending platform work: Keychain bridge
(`internal/crypto.PlatformBridge`), camera frame source
(`ui/widgets.FrameSource`), APNs registration (Phase 5).

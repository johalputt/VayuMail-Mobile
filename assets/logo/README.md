# VayuMail logo system

## Concept

Vayu (वायु) means wind, air, breath in Sanskrit — the invisible carrier
that moves things without being seen. The mark is a **"vy" ligature**: a
short left arm meets a longer right arm that curves down-left into a
y-tail — the initials of *vayu* fused into a single rounded glyph. One
weight, no ornament — it reads as motion and arrival at once.

The mark deliberately avoids every exhausted mail-app symbol: no envelope,
no padlock, no paper plane, no speech bubble, no gradient, no skeuomorphic
treatment of any kind.

The wordmark pairs the mark with **vayumail** in a rounded geometric sans
so the name reads as one word.

## Files

These are the original master artworks (PNG, 500×500, transparent
background). They are used verbatim — the app does not redraw them.

| File | Use |
|---|---|
| `vayumail.png` | Full logo (mark + wordmark), black — light backgrounds |
| `vayumail-dark.png` | Full logo, white — dark backgrounds |

Derived, in-repo:

| File | Source |
|---|---|
| `cmd/vayumail/appicon.png` | The mark from `vayumail.png`, centered on an opaque white square — the launcher icon gogio embeds in the APK/IPA. |
| `ui/logo-light.png` | A copy of `vayumail.png`, embedded via `go:embed` and painted on the in-app splash. |

The splash shows the logo **statically** — no animation.

## Color

| Context | Value |
|---|---|
| Light backgrounds | black (`#0D0D0D`) |
| Dark backgrounds | white (`#F5F5F5`) |
| Notification icon (Android status bar) | `#FFFFFF` |

The logo is monochrome in every context. Never apply the Accent blue, a
gradient, or a shadow to any logo asset.

## Regenerating the derived assets

If the master `vayumail.png` changes, regenerate the launcher icon and the
embedded splash copy from it (any image tool works); keep the mark crop
tight and centered, and keep `ui/logo-light.png` identical to
`vayumail.png`.

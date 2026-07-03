# VayuMail logo system

## Concept

Vayu (वायु) means wind, air, breath in Sanskrit — the invisible carrier
that moves things without being seen. The mark is a **"vy" ligature**
drawn as if by a gust: a short left arm meets a longer right arm that
curves down-left into a y-tail — the initials of *vayu* fused into a
single glyph. Rounded caps, one weight, no ornament — it reads as motion
and arrival at once.

The mark deliberately avoids every exhausted mail-app symbol: no envelope,
no padlock, no paper plane, no speech bubble, no gradient, no skeuomorphic
treatment of any kind.

The wordmark pairs the mark with **vayumail** set in a rounded geometric
sans, the two syllables distinguished only by weight — regular `vayu`,
semibold `mail` — so the name reads as one word with an internal rhythm.

## Files

| File | Use |
|---|---|
| `vayumail-icon.svg` | Icon only — app icon, favicon, badge |
| `vayumail.svg` | Full wordmark, light backgrounds |
| `vayumail-dark.svg` | Full wordmark, dark backgrounds |

The launcher icon `cmd/vayumail/appicon.png` and the in-app splash mark
(`ui/boot.go`) are both generated from the icon geometry below, so the
brand is pixel-consistent from the store listing to the running app. The
splash shows the mark statically — no animation.

## Geometry (do not alter)

On a 64×64 canvas, stroke width 11, round caps/joins:

```
left arm:   M 19 17 L 31 40
right arm:  M 45 17 C 43 31, 37 44, 27 51
```

`tools/genicon` rasterizes exactly this into the launcher PNG (pure Go,
reproducible in CI). `ui/boot.go` strokes the same two paths on device.
Any geometry change is a redesign and must update all three in lockstep:
the SVGs, `tools/genicon`, and `ui/boot.go`.

## Color values

| Context | Value |
|---|---|
| Light backgrounds (stroke/fill) | `#0D0D0D` |
| Dark backgrounds (stroke/fill) | `#F5F5F5` |
| Notification icon (Android status bar) | `#FFFFFF` |

The mark is monochrome in every context. Never apply the Accent blue, a
gradient, or a shadow to any logo asset.

## Export pipeline

The launcher icon is generated in-repo (no external toolchain):

```sh
go run ./tools/genicon -size 1024 -o AppIcon-1024.png   # iOS
go run ./tools/genicon -size 432  -o ic_launcher_fg.png  # Android adaptive fg
go run ./tools/genicon                                   # cmd/vayumail/appicon.png (512)
```

For SVG rasterization elsewhere, `rsvg-convert` works too:

```sh
rsvg-convert -w 32 -h 32 vayumail-icon.svg -o favicon.png
```

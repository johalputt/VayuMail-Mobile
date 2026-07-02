# VayuMail logo system

## Concept

Vayu (वायु) means wind, air, breath in Sanskrit — the invisible carrier that
moves things without being seen. The mark is two open arcs that converge at
a single point: airflow, signal, transmission — arrival without a container.
A V drawn by the wind, not by a ruler.

The arcs are intentionally asymmetric in curvature — the left arc curves
slightly more aggressively than the right, implying directionality without
motion arrows. This is subtle and intentional.

The mark deliberately avoids every exhausted mail-app symbol: no envelope,
no padlock, no paper plane, no speech bubble, no gradient, no skeuomorphic
treatment of any kind.

The wordmark articulates the name through weight alone: light "vayu"
(weight 300), medium "mail" (weight 500). No separator, no color
difference, no spacing gap — the two words read as one mark with an
internal rhythm.

## Files

| File | Use |
|---|---|
| `vayumail-icon.svg` | Icon only — app icon, favicon, badge |
| `vayumail.svg` | Full wordmark, light backgrounds |
| `vayumail-dark.svg` | Full wordmark, dark backgrounds |

## Color values

| Context | Value |
|---|---|
| Light backgrounds (icon and wordmark stroke/fill) | `#0D0D0D` |
| Dark backgrounds (stroke/fill) | `#F5F5F5` |
| Notification icon (Android status bar) | `#FFFFFF` |

The mark is monochrome in every context. Never apply the Accent blue, a
gradient, or a shadow to any logo asset.

## Do not modify geometry

The arc paths, convergence dot, canvas proportions, and the wordmark's
weight split are fixed. Recoloring for the contexts listed above is the
only permitted modification. Any geometry change is a redesign and requires
maintainer sign-off.

## Export pipeline

Rasterize with `rsvg-convert` (librsvg):

```sh
# iOS app icon 1024x1024
rsvg-convert -w 1024 -h 1024 vayumail-icon.svg -o AppIcon-1024.png

# Android adaptive icon foreground (108x108pt at 3x = 324px)
rsvg-convert -w 324 -h 324 vayumail-icon.svg -o ic_launcher_foreground.png

# Android notification icon 24x24pt at 3x = 72px, white variant:
# first change every stroke/fill to #FFFFFF in a temporary copy, then
rsvg-convert -w 72 -h 72 vayumail-icon-white.svg -o ic_stat_vayumail.png

# Favicon 32x32
rsvg-convert -w 32 -h 32 vayumail-icon.svg -o favicon.png
```

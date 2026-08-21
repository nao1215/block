#!/usr/bin/env python3
"""Draw doc/img/favicon.png, the browser-tab icon for the documentation site.

The README and the front page show a recorded terminal; a favicon cannot. A
64-pixel tab icon has to be a shape, so this draws one — a padlock whose body
is a stack of three blocks — rather than shrinking a screenshot into a grey
smudge.

It is generated rather than hand-drawn only so the shape is readable and can
be re-rendered at any size. Replace doc/img/favicon.png with any square image
and the site picks it up; nothing else refers to this script.

    python3 scripts/gen-favicon.py     # or: make favicon
"""

import os
import sys

from PIL import Image, ImageDraw

BG = (13, 17, 23)  # the same ground the documentation site paints
ACCENT = (88, 166, 255)

SCALE = 4  # draw oversampled, then downsample: PIL has no antialiased shapes


def draw_mark(d, x, y, w, h):
    """A padlock whose body is three stacked blocks."""
    # Shackle, behind and above the body: an arch with two legs running down
    # into it. Narrower than the body, or the whole mark reads as a basket.
    sw = w * 0.13
    arch_w = w * 0.52
    ax = x + (w - arch_w) / 2
    arch_h = h * 0.30
    body_top = y + h * 0.38
    d.arc(
        [ax, y + sw / 2, ax + arch_w, y + sw / 2 + arch_h * 2],
        start=180,
        end=360,
        fill=ACCENT,
        width=int(sw),
    )
    for side in (ax + sw / 2, ax + arch_w - sw / 2):
        d.line([side, y + sw / 2 + arch_h, side, body_top + sw], fill=ACCENT, width=int(sw))

    # Body: one solid block with two slots milled out, so it reads as three
    # bars locked together rather than three loose ones.
    d.rounded_rectangle([x, body_top, x + w, y + h], radius=w * 0.10, fill=ACCENT)
    bar_h = (y + h - body_top) / 5.0
    for i in (1, 3):
        top = body_top + bar_h * i
        d.rounded_rectangle(
            [x + w * 0.14, top, x + w * 0.86, top + bar_h],
            radius=bar_h * 0.42,
            fill=BG,
        )


def main():
    size = 512
    S = size * SCALE
    im = Image.new("RGB", (S, S), BG)
    d = ImageDraw.Draw(im)
    h = int(S * 0.74)
    w = int(h * 0.80)
    draw_mark(d, (S - w) // 2, (S - h) // 2, w, h)

    root = os.path.join(os.path.dirname(os.path.abspath(__file__)), os.pardir)
    out = os.path.join(root, "doc", "img", "favicon.png")
    os.makedirs(os.path.dirname(out), exist_ok=True)
    im.resize((size, size), Image.LANCZOS).save(out, optimize=True)
    print(f"wrote {out} ({size}x{size})")
    return 0


if __name__ == "__main__":
    sys.exit(main())

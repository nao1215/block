#!/usr/bin/env python3
"""Draw the block wordmark into doc/img/.

The logo is generated rather than hand-edited so it stays reproducible and can
be re-rendered at any size: run "make logo" after changing anything here.

The mark is a padlock whose body is a stack of three blocks, and the wordmark
is set in Hack — the font a terminal uses, because everything block does
happens at one.
"""

import os
import sys

from PIL import Image, ImageDraw, ImageFont

BG = (13, 17, 23)  # GitHub dark, the same ground the documentation site paints
FG = (230, 237, 243)
ACCENT = (88, 166, 255)

HACK_BOLD = "/usr/share/fonts/truetype/hack/Hack-Bold.ttf"

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
    body = [x, body_top, x + w, y + h]
    d.rounded_rectangle(body, radius=w * 0.10, fill=ACCENT)
    bar_h = (y + h - body_top) / 5.0
    for i in (1, 3):
        top = body_top + bar_h * i
        d.rounded_rectangle(
            [x + w * 0.14, top, x + w * 0.86, top + bar_h],
            radius=bar_h * 0.42,
            fill=BG,
        )


def render_mark(size, out):
    """The mark on its own, square — what a 64x64 favicon can actually show."""
    S = size * SCALE
    im = Image.new("RGB", (S, S), BG)
    d = ImageDraw.Draw(im)
    h = int(S * 0.74)
    w = int(h * 0.80)
    draw_mark(d, (S - w) // 2, (S - h) // 2, w, h)
    im.resize((size, size), Image.LANCZOS).save(out, optimize=True)
    print(f"wrote {out} ({size}x{size})")


def render(width, height, out):
    W, H = width * SCALE, height * SCALE
    im = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(im)

    mark_h = int(H * 0.70)
    mark_w = int(mark_h * 0.80)
    gap = int(H * 0.16)
    word = ImageFont.truetype(HACK_BOLD, int(H * 0.58))
    box = d.textbbox((0, 0), "block", font=word)
    total = mark_w + gap + (box[2] - box[0])

    mx = (W - total) // 2
    my = (H - mark_h) // 2
    draw_mark(d, mx, my, mark_w, mark_h)
    d.text(
        (mx + mark_w + gap - box[0], (H - (box[3] + box[1])) // 2),
        "block",
        font=word,
        fill=FG,
    )

    im.resize((width, height), Image.LANCZOS).save(out, optimize=True)
    print(f"wrote {out} ({width}x{height})")


def main():
    root = os.path.join(os.path.dirname(os.path.abspath(__file__)), os.pardir)
    img = os.path.join(root, "doc", "img")
    os.makedirs(img, exist_ok=True)
    render(1800, 600, os.path.join(img, "block-logo.png"))
    render_mark(512, os.path.join(img, "block-mark.png"))
    return 0


if __name__ == "__main__":
    sys.exit(main())

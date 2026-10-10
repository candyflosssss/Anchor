#!/usr/bin/env python3
"""Reproduce the checked-in PNG/ICO from the Lucide Anchor geometry in anchor.svg.
Requires Pillow only for regeneration, not for builds or application runtime.
"""
from pathlib import Path
import math
from PIL import Image, ImageDraw

root = Path(__file__).resolve().parent
scale = 16
image = Image.new("RGBA", (64 * scale, 64 * scale))
draw = ImageDraw.Draw(image)
draw.rounded_rectangle((0, 0, 64 * scale - 1, 64 * scale - 1), radius=14 * scale, fill="#171717")
width = round(1.75 * 1.5 * scale)

def point(x, y):
    return (round((14 + 1.5 * x) * scale), round((14 + 1.5 * y) * scale))

def line(points):
    points = [point(x, y) for x, y in points]
    draw.line(points, fill="#fafafa", width=width, joint="curve")
    for x, y in (points[0], points[-1]):
        r = width / 2
        draw.ellipse((x-r, y-r, x+r, y+r), fill="#fafafa")

line([(12, 6), (12, 22)])
line([(19, 13), (21, 12)] + [(12 + 9 * math.cos(t * math.pi / 180), 12 + 9 * math.sin(t * math.pi / 180)) for t in range(181)] + [(5, 13)])
line([(9, 11), (15, 11)])
x, y = point(12, 4)
r = 3 * scale
# Center the stroke on the SVG circle, not just inside its bounding box.
half = width / 2
draw.ellipse((x-r-half, y-r-half, x+r+half, y+r+half), fill="#fafafa")
draw.ellipse((x-r+half, y-r+half, x+r-half, y+r-half), fill="#171717")
image = image.resize((256, 256), Image.Resampling.LANCZOS)
image.save(root / "anchor.png")
image.save(root / "anchor.ico", sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])

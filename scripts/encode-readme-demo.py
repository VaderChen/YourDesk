"""Encode captured UI frames with one palette and verify the looping GIF."""
import json
from pathlib import Path
import sys
from PIL import Image

folder, output = map(Path, sys.argv[1:])
manifest = json.loads((folder / 'frames.json').read_text())
frames = [Image.open(folder / item['name']).convert('RGB') for item in manifest]
assert frames and all(frame.size == (1100, 760) for frame in frames)
palette_sheet = Image.new('RGB', (160, 112 * len(frames)))
for index, frame in enumerate(frames):
    palette_sheet.paste(frame.resize((160, 112)), (0, 112 * index))
palette = palette_sheet.quantize(colors=248)
# Reserve small, saturated UI details so the cursor/status colors survive
# alongside the many neutral shades in dialog backdrops and text edges.
accents = [(37, 99, 235), (34, 197, 94), (0, 255, 0), (239, 68, 68),
           (220, 38, 38), (23, 108, 80), (255, 255, 255), (15, 23, 42)]
palette.putpalette(palette.getpalette()[:248 * 3] + [v for color in accents for v in color])
indexed = [frame.quantize(palette=palette, dither=Image.Dither.NONE) for frame in frames]
candidate = output.with_suffix('.candidate.gif')
indexed[0].save(candidate, save_all=True, append_images=indexed[1:],
                duration=[item['duration'] for item in manifest], loop=0,
                optimize=True, disposal=1)
with Image.open(candidate) as result:
    assert result.n_frames > 40 and result.info['loop'] == 0
    duration = 0
    for index in range(result.n_frames):
        result.seek(index)
        assert result.size == (1100, 760)
        result.load()
        duration += result.info['duration']
    assert 10000 <= duration <= 60000
assert candidate.stat().st_size < 5 * 1024 * 1024
candidate.replace(output)
print(f'GIF verified: {len(frames)} source frames, {duration / 1000:.1f}s, {output.stat().st_size / 1024:.0f} KiB')

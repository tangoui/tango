#!/usr/bin/env python3
"""Subset the OFL-licensed Noto Sans SC for demo/locale text. Optional fontTools
build tool only: normal documentation builds use the checked-in subset.
Usage: python subset-gallery-font.py /path/to/NotoSansSC[wght].ttf
"""
import hashlib
from pathlib import Path
import sys
from fontTools import subset
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont

repo = Path(__file__).resolve().parents[2]
source = Path(sys.argv[1])
characters = {ord(c) for directory in [repo/'ui', repo/'examples/ui-gallery']
              for path in directory.rglob('*.go') for c in path.read_text()
              if ord(c) > 127}
font = TTFont(source, recalcTimestamp=False)
options = subset.Options()
options.recalc_timestamp = False
subsetter = subset.Subsetter(options=options)
subsetter.populate(unicodes=characters)
subsetter.subset(font)
if 'fvar' in font:
    font = instantiateVariableFont(font, {'wght':400}, inplace=True)
output = repo/'site/tools/playground/fonts/NotoSansSC-demo.ttf'
output.parent.mkdir(parents=True, exist_ok=True)
font.save(output)
print(f'{len(characters)} requested glyphs; font subset {output.stat().st_size} bytes')
print('Source sha256:', hashlib.sha256(source.read_bytes()).hexdigest())

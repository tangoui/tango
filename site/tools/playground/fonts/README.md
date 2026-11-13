# Noto Sans SC demo subset

Source: Google Fonts, `ofl/notosanssc/NotoSansSC[wght].ttf` in the official
`google/fonts` repository. The source file SHA-256 is
`a3041811a78c361b1de50f953c805e0244951c21c5bd412f7232ef0d899af0da`.

License: SIL Open Font License 1.1, reproduced in `OFL.txt` and the public
playground's `NOTO-SANS-SC-LICENSE.txt`. The font has been subset and
instantiated at weight 400; this is not the full upstream distribution.
The subset contains non-ASCII glyphs used in `ui/**/*.go` and
`examples/ui-gallery/**/*.go` at generation time (1610 requested code points).
Go Regular remains the Latin fallback. Arbitrary additional characters can
be provided through the preview's local font import.

Reproduce using Python with fontTools 4.68.0 installed:

```bash
python3 site/tools/subset-gallery-font.py '/path/to/NotoSansSC[wght].ttf'
```

Normal documentation builds use the checked-in 422 KiB subset and require
neither Python nor a font service. Regenerate it when new demo labels use
additional glyphs, retaining the accompanying license.

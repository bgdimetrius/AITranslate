# Small promo tile design brief

## Placement and viewing conditions

- Source asset: 440 × 280 pixels (11:7), PNG or JPEG.
- Current Chrome Web Store search results render the asset at approximately 275 × 175 pixels (62.5% scale).
- Google explicitly requires the tile to remain effective at half size: 220 × 140 pixels.
- The tile sits on a pale gray store surface, so the artwork needs a full-bleed background and a defined outer silhouette.

## Current-market benchmark

Observed on 2026-11-09 from the first eight image-bearing results for `translation` in the live Chrome Web Store:

- 8/8 include the extension name or brand inside the tile.
- 0/8 use a full product screenshot.
- 1/8 uses a compact UI fragment; the rest use a logo, symbol, illustration, or benefit-led composition.
- 6/8 keep the prominent copy to roughly six words or fewer.
- 4/8 use a saturated full-bleed background; light tiles compensate with a large, high-contrast brand mark.

This is an observational benchmark, not a performance or conversion study. It establishes the visual conventions users see beside this extension.

## Research-backed principles

1. **One message and one focal point.** Visual hierarchy should make the most important element the first thing seen, using scale, contrast, and spacing.
2. **Three-second comprehension.** The benefit and visual should communicate the purpose immediately; if they do not, remove competing elements.
3. **Brand card, not screenshot.** Chrome says promotional images should communicate the brand, remain uncluttered, use little text, fill the region, and work at half size.
4. **Minimize overlaid content.** Google Ads recommends keeping overlaid text and logos from dominating the image and cites less than 20% image coverage as a useful ceiling for overlaid elements.
5. **Accessible contrast.** Target at least 4.5:1 for small text and 3:1 for large text or images of text.
6. **No fake controls.** A promo tile is already clickable in its store context; adding button-like controls creates a misleading interaction cue.

## Measurable acceptance criteria

| Property | Target |
| --- | --- |
| Prominent words | 2–6 total |
| Primary messages | 1 |
| Hero visuals | 1 |
| Full-size outer padding | 24–32 px |
| Headline size | 34–40 px at source |
| Headline at store display | At least 21 px |
| Headline at half size | At least 17 px |
| Small-text contrast | At least 4.5:1 |
| Large-text contrast | At least 3:1 |
| Fonts | 1 family, 2 weights maximum |
| Main palette | 2 brand hues plus neutrals |
| Full UI screenshots | 0 |
| Fake CTA buttons | 0 |
| Final export | 440 × 280, RGB PNG, no alpha |

## Message strategy for AI Translator

The extension has two functional modes: longer-form translation in its popup and in-page translation or contextual interpretation of selected words and phrases. The audience is language learners reading online. The strongest differentiated benefit is therefore not merely “translate text,” but **understand what you read**.

The final tile uses:

- brand: `AI Translator`;
- benefit: `Understand what you read.`;
- hero: the extension’s translation mark;
- abstract reading lines rather than a literal UI screenshot.

## Sources

- [Chrome Web Store: Creating a great listing page](https://developer.chrome.com/docs/webstore/best-listing)
- [Chrome Web Store: Supplying images](https://developer.chrome.com/docs/webstore/images)
- [Chrome Web Store live translation search](https://chromewebstore.google.com/search/translation)
- [Google Ads: Make better creative assets for App campaigns](https://support.google.com/google-ads/answer/6167158)
- [W3C: Understanding minimum text contrast](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)
- [Nielsen Norman Group: Visual hierarchy in UX](https://www.nngroup.com/articles/visual-hierarchy-ux-definition/)
- [Adobe: Basic principles of graphic design](https://www.adobe.com/learn/express/web/graphic-design-basics)
- [Adobe: Common graphic-design mistakes](https://www.adobe.com/express/learn/blog/common-graphic-design-mistakes)

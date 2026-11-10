# Marquee promo tile design brief

## Placement and viewing conditions

- Source asset: 1400 × 560 pixels (5:2), JPEG or 24-bit PNG without alpha.
- Chrome describes the marquee as an optional asset used for more prominent featuring.
- The asset must still work at half size, so the primary idea is tested at 700 × 280 pixels.
- Current Chrome Web Store featured slides can pair imagery with store-generated titles, descriptions, ratings, and controls. Essential artwork therefore stays away from the extreme edges and does not depend on a built-in CTA.
- Promotional images are not locale-specific. Text is kept short, while the functional story remains understandable from the selection highlight and meaning card.

## Live-store observations

Observed on 2026-11-09 from the current Chrome Web Store Discover carousel:

- Featured collection slides use one dominant illustration, a saturated full-bleed palette, a short heading, and generous negative space.
- The individual-extension slide separates product imagery from store-generated metadata and CTA.
- The carousel is displayed inside a rounded container and can be partially obscured by browser prompts or store controls.
- The current individual-extension example uses a product-in-use visual rather than an icon-only brand card.

These are placement observations, not conversion-performance claims. The marquee asset remains useful even though Chrome can vary how promotional artwork is surfaced.

## Research-backed principles applied

1. **Brand first, product second.** Chrome says promo images should primarily communicate the brand rather than simply reproduce a screenshot.
2. **One visual story.** The selection highlight, cursor, and contextual-meaning card form a single cause-and-effect sequence.
3. **Half-size legibility.** The three-word headline and primary product shapes survive at 50% scale; small UI copy is supportive rather than essential.
4. **Safe composition.** Logo, headline, selection, and meaning card remain within roughly the central 86% of the canvas.
5. **Full bleed and strong edges.** Saturated blue fills the canvas and contrasts with the light-gray store surface.
6. **Minimal overlay and no CTA.** Copy is concise, negative space is preserved, and the artwork contains no fake button.
7. **Distinct from the small tile.** The small tile is a compact brand card; the marquee demonstrates the extension’s core interaction.

## Message strategy

The extension translates longer text in its popup and gives in-page contextual interpretations for selected words and phrases. The final marquee focuses on that differentiated in-page learning loop without a written headline: an abstract reading page sends a selected phrase through the extension mark into a layered contextual result.

The textless, cause-and-effect composition follows Chrome's advice to avoid text, remains language-neutral, and does not duplicate the small tile's brand-card layout.

## Acceptance criteria

| Property | Target |
| --- | --- |
| Canvas | 1400 × 560 pixels |
| Final format | RGB PNG, no alpha |
| Primary promise | Visual, no headline |
| Hero stories | 1 |
| Fake CTA buttons | 0 |
| Essential safe area | Central ~86% |
| Half-size test | 700 × 280 pixels |
| Main palette | 2 brand blues plus neutrals |
| Fonts | Inter, 3 weights maximum |
| Full product screenshots | 0 |

## Sources

- [Chrome Web Store: Supplying images](https://developer.chrome.com/docs/webstore/images)
- [Chrome Web Store: Creating a compelling listing](https://developer.chrome.com/docs/webstore/best-listing)
- [Live Chrome Web Store Discover carousel](https://chromewebstore.google.com/)
- [Google: Creative best-practices checklist](https://services.google.com/fh/files/misc/creative_best_practices_checklist.pdf)
- [Think with Google: Build better creative for performance marketing](https://www.thinkwithgoogle.com/_qs/documents/12245/Build_better_creative_for_your_performance_marketing_2.pdf)
- [W3C: Understanding minimum text contrast](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)

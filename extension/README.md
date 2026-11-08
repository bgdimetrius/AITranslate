# AI Translate browser extension

The extension is built with WXT, Vue 3, and Manifest V3.

## Local checks

```sh
npm ci
npm run compile
npm run package:chrome
```

`package:chrome` creates the Chrome Web Store-compatible ZIP under `.output/`.

## Latest committed build

Every successful Extension CI run for an extension change pushed to `main` is packaged and published
by Extension CD to the private release host. Open the release page, enter the access key, and the
latest successful extension package starts downloading:

<https://releases.ai-translate.bogdanantonovich.com/>

The server retains only the newest package. Anonymous or incorrect-key requests for the ZIP receive
`404`. Scripts can use `Authorization: Bearer ACCESS_KEY` with the protected direct URL:

```text
https://releases.ai-translate.bogdanantonovich.com/latest/ai-translate-extension.zip
```

This is a downloadable extension package, not an automatic Chrome Web Store submission. Store
submission still requires the extension's Chrome Web Store account and listing credentials.

# MyGo used by the desktop client

This standalone module is based on `github.com/egoist/mygo` **v0.10.0**, upstream
commit `ebde9399baf2734b3361856a53f4036859973d1d`. Its original **MIT** license is
preserved in `LICENSE`. Imports retain their upstream module path.

The tree includes the runtime, UI, CLI, platform code, plugins, generators,
locales, embedded resources and their test fixtures. Upstream website, examples
and release packaging are outside this source copy.

Local changes:

- `PageOptions.ResourceFilter` and `ResourceHandler` let Windows WebView2
  answer selected HTTP(S) resources from a local frontend bundle without
  changing the website origin or its persistent session. It reuses deferred,
  streaming protocol responses; pages without these options keep their normal
  network path. Other backends continue using the network. The opt-in filter,
  window isolation, original URL and complete binary response have unit tests.

- Windows tracks each pending `NavigateToString` document by its internal data
  URL. Consecutive `LoadHTML` calls remain application navigations when one
  replaces another before loading, without bypassing page navigation policy.

- `Page.SetPermissionRequestHandler` lets a native permission prompt respond
  asynchronously, from any goroutine, while retaining `SetPermissionHandler`.
  Windows, macOS and Linux retain each native request until answered once;
  closing its window or embedded web view denies and releases pending requests.

- Windows WebView2 downloads and permission requests hold event deferrals while
  their handlers run from the main-thread queue. Save and permission dialogs
  open after the COM callback returns; closed windows cancel or deny pending
  requests. Download completion handlers also run from the main-thread queue.

- `Element.ScrollViewport()` exposes the nearest scroll ancestor's transformed
  inner viewport for controls that follow nested and modal scrolling.

- Inline paint scopes share the paragraph's original shaping and selection.
  Blur, effects, opacity, clips, masks, Draw/DrawOver callbacks, glyphs, text
  backgrounds, decorations and inline objects compose in the same nested scope.
  Opacity applies once to a group's completed image. Plain paragraphs retain
  their direct paint path; scope scratch space and layer canvases are reused.
- Original RichText spans are kept separately from flattened layout spans so
  painting can preserve color alpha independently of element opacity, including
  empty spans and retained transition trees.
- Inline `NoWrap()` keeps all styled descendants in one atomic range while
  preserving their text, objects and interaction. Explicit newlines still break;
  an oversized range may overflow its line.
- `Ruby(annotation)` retains the styled base in the original rune stream while
  laying out a half-size annotation, typographic-unit spacing and adjacent text
  overhang. Nested ruby, inline blocks and preceding-line space participate in
  its metrics. Windows reads OpenType OS/2 em metrics through DirectWrite;
  platforms without those metrics normalize the available ascent/descent.
  Geometry regressions use Chromium 156 with MiSans at scales 1, 1.25, 1.5 and 2.
  `ui/testdata/ruby_chromium.html` reproduces the local browser measurements.
- `InlineBlock()` gives a rich inline subtree its own shrink-to-fit paragraph,
  aligned to its last baseline. `WrapAnywhere()` reduces its minimum width to a
  grapheme. Nested boxes retain original text offsets, links, object callbacks,
  padding, borders, transforms, selection and caret navigation.
- `PreWrap()` preserves newlines and selectable hanging whitespace without
  adding trailing spaces to intrinsic/alignment width. Inline blocks inherit
  the whitespace policy. `TextShape.PreWrap` and `TextShape.TabSize` expose the
  same shaping rules to custom text controls. Existing `PreserveSpaces()`
  behavior is retained.
- `Element.ResolvedText() (Font, Color)` exposes the existing inherited text
  style for custom inline drawing. Element opacity belongs to the paint scope.
- Overlay nodes resolve the root's inherited text style while their view is
  built, retaining their final ordering above window content. Controls that size
  icons from `ResolvedText()` receive the same font used for layout and painting.
  Overlays created in a `Themed` scope capture that theme when each top-level
  node is created, preserving nested scopes and explicit font/color overrides.
- `Element.WordBreakAll()` and `TextShape.WordBreakAll` expose CSS word breaking
  within words, retaining grapheme clusters, punctuation rules, atomic inline
  ranges and original glyph/selection offsets. Layout and shape cache keys
  include the policy; inline blocks retain it in their child paragraphs.
- `Element.TextLineCount()` reports the last laid-out paragraph's line count
  and truncation, allowing application-owned block line budgets to share native
  rich text shaping without replacing its original text or links.
- `Element.TextGradient(LinearGradient)` fills inherited glyphs directly with
  gradient stops. Backgrounds, images, borders, shadows and decoration colors
  retain their own painting; nested group opacity remains shared.
- `FontMetrics.XHeight` exposes available platform font metrics without changing
  ascent, descent, line gap or layout.
- `Span.LineAscent` and `Span.LineDescent` preserve explicit inline line boxes
  when mixing CSS font families. They retain original glyphs, selection and
  links, compose with baseline shifts and inline containers, and leave zero-
  valued spans on the existing font-metric path.
- `Context.PixelScale()` exposes the current display scale multiplied by window
  zoom, for CSS-compatible pixel rounding without changing default border rules.
- Translucent glyphs and opacity/blur/effect layers use grayscale coverage;
  opaque text retains LCD rendering. This prevents color fringes while fading.
- Windows Media Foundation receives decoded UTF-16 filesystem paths for local
  `file:` URLs, retaining network URL escaping. Local playback supports Unicode,
  spaces and literal percent/hash filename characters through Open and OpenURL.
- Media Foundation primes the initial video picture before starting the playback
  clock. A seekable source whose decoder prerolls at a later keyframe is positioned
  at its start before exposing the image. Initial pause/seek requests and audio-only
  playback are preserved. During an initial paused seek, the player schedules
  stream ticks until Media Foundation reports that the seek has landed, so a
  window waiting for its first picture continues to receive frames.
- `VideoPlayer.Buffered()` exposes actual downloaded time ranges. On Windows,
  Media Foundation's `IMFMediaEngine.GetBuffered` supplies these intervals.
- `VideoPlayer.HasAudio()` reports audio-track presence separately from metadata
  readiness. Media Foundation, AVFoundation and GStreamer query the loaded
  resource's tracks; browsers use exposed track metadata and return unknown
  when it is unavailable. Image-only decoders report no audio. Muting, volume,
  silence and PCM analysis availability do not change the result.
- `VideoPlayer.AudioSamples()` exposes timestamped mono PCM at the playback
  position, before volume/mute. A synchronous passthrough Media Foundation
  transform observes decoded samples while retaining the renderer's original
  samples, clock and audio output. Its bounded sample history flushes on seek;
  PCM integer/float and standard speaker down-mixing are supported. Engines
  without analysis report unavailable without changing playback.

- `Span.ExactLineBox` opts explicit ascent/descent into CSS inline line boxes,
  preserving glyph size, fallback emoji, caret offsets and empty lines. Ordinary
  spans and ruby keep their existing font-extent reservation behavior. Regression
  coverage includes wrapped MiSans chat previews and mixed explicit/default spans.
- `ObserveInput` retains the original ancestor chain for a pressed pointer's
  moves and release, including middle-button and touch drags outside the region.
  Pointer device/pressure and cancellation remain attached to each event.
- `ObserveWindowPointer` passively forwards window pointer movement to mounted
  elements without consuming input or requesting a native rebuild. Embedded
  mascot scenes use it for their original cursor-following protocol.
- `WebViewPointerEvents(false)` keeps an embedded page visible and focusable
  while pointer and scrolling pass to native UI, matching iframe pointer-events.
- `ListState.RetainAll` keeps every keyed row mounted when viewport culling is
  disabled by the application's render preference. Changing it preserves the
  list's scroll state and restores visible-row mounting when switched off.

- `StateScope` and `StateRoot` isolate page-owned local state, including deferred
  virtual-list rows and overlays. Portal identities remain stable when another
  page opens or closes an overlay. `KeepAlive` retains inactive page editor,
  selection, scroll and resource lifetimes, restores input focus on return,
  and releases the retained subtree once its host evicts it.

- `InputLineBox(ascent, descent)` gives an editable input explicit line metrics
  for the current frame. Empty lines, wrapping, IME composition, caret positions
  and auto-height use the same box, while glyph sizes remain unchanged. Omitted
  calls retain the existing font metrics. This aligns the native chat textarea
  with Chromium's device-pixel-rounded MiSans `normal` line height.

Regression commands, from this directory:

```sh
go generate ./ui
go test ./ui ./internal/text -count=1
go test ./ui/video -count=1
GOOS=linux GOARCH=amd64 go test -c ./ui -o /tmp/mygo-ui-linux.test
GOOS=darwin GOARCH=arm64 go test -c ./ui -o /tmp/mygo-ui-darwin.test
GOOS=js GOARCH=wasm go test -c ./ui -o /tmp/mygo-ui-wasm.test
```

The inline scope tests compare actual rendered pixels, including text/image
overlap, nested opacity, transformed clipping, sharp adjacent text and custom
draw callbacks. Inline range tests cover CJK, bidi, newlines, objects, links and
selection offsets. Upgrades should retain these regressions and regenerate the
public facade in `ui/api_gen.go`.

## WebView2 runtime preflight

`CheckWebViewRuntime` exposes the existing runtime finder without starting a browser. Camp uses it for the native missing-runtime guide and installer exit-code check. Regression: `TestCheckWebViewRuntimeRejectsMissingFixedRuntime`.

- `PageOptions.PersistentStorageOrigin` grants WebView2 storage persistence to
  the configured origin before its first navigation using the profile permission API.

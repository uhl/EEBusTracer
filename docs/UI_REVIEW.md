# UI Review — Deferred Items

Issues identified during the toolbar unification review, to be tackled in
follow-up iterations.

## Medium Priority (Done)

### ~~Too much vertical chrome (138px before first message row)~~
- ~~Capture controls auto-collapse on trace pages when idle; compact "Capture"
  button expands them on demand~~

### ~~Gear icon means "settings", not "presets"~~
- ~~Replaced with star icon; added delete button (hover to reveal ×) on each
  preset item~~

### ~~No sequence number in the message table~~
- ~~Added `#` column showing SequenceNum~~

### ~~No classifier color-coding in message rows~~
- ~~Classifier cells color-coded: read (teal), reply (green), write (amber),
  call (purple), notify (blue)~~

## Low Priority

### Ctrl+L / Ctrl+G browser shortcut conflicts
- `Ctrl+L` is "focus address bar" in all major browsers; our handler may lose
  the fight against browser chrome
- `Ctrl+G` conflicts with "Find next" (Firefox) and "Go to line" (DevTools)
- No shortcut to focus the filter text input (arguably the most-used control)
- Consider `Ctrl+Shift+F` for filter focus, or use the `/` key (vim idiom,
  already used by GitHub)

### Noise overlay GPU cost
- Full-viewport SVG fractal noise at `z-index: 9999`, rendered every frame
- ~2–5% GPU overhead on integrated graphics / battery laptops
- No `prefers-reduced-motion` check to disable it
- Consider disabling on battery or behind a setting

### Entrance animations ignore prefers-reduced-motion
- `revealUp 0.4s` with staggered delays fires on every page load
- For a tool used dozens of times per day, animations become friction
- Add `@media (prefers-reduced-motion: reduce) { .reveal { animation: none; } }`

### Toolbar divider breaks on flex-wrap
- The 1px × 20px divider between Filter and Find sections doesn't make sense
  when the toolbar wraps on narrow viewports
- Consider hiding the divider when wrapped, or using a different separator
  strategy (e.g. background color difference)

# Adding Overview Tab Renderers

The Overview tab uses a registry pattern to dispatch rendering for each SPINE
function set. Adding support for a new function set requires **one registration
call** and **one render function** -- no switch statements, no manual HTML.

## Quick reference

| What you need | Function |
|---|---|
| Register by exact function set name | `registerOverview(name, opts)` |
| Register by substring/pattern match | `registerOverviewPattern(matchFn, opts)` |
| Render a data table | `overviewTable(title, columns, rows, opts?)` |
| Render key-value pairs | `overviewKV(pairs)` |
| Format a SPINE ScaledNumber | `fmtScaled(scaledNumber, unit?)` |

All code lives in `web/static/js/app.js`, inside the Overview Registry section.

---

## Step-by-step: table renderer

Suppose you want to add an overview for `IncentiveTableData`.

### 1. Write the render function

The function receives `(cmds, descs)`:

- **`cmds`** -- the SPINE `cmd` array extracted from the datagram payload.
  Iterate over it and dig into the function-set-specific key.
- **`descs`** -- the description context object (measurements, limits, etc.).
  Only populated when `needsDescs: true`; otherwise `null`.

```js
function renderIncentiveTableOverview(cmds) {
    var rows = [];
    for (var i = 0; i < cmds.length; i++) {
        var cmd = cmds[i];
        if (!cmd.incentiveTableData) continue;
        var slots = cmd.incentiveTableData.incentiveSlot || [];
        for (var j = 0; j < slots.length; j++) {
            var s = slots[j];
            rows.push({
                Tier:     String(s.tier || ''),
                Price:    fmtScaled(s.price, s.unit),
                Duration: String(s.duration || '')
            });
        }
    }
    return overviewTable('Incentive Table', ['Tier', 'Price', 'Duration'], rows);
}
```

### 2. Register it

```js
registerOverview('IncentiveTableData', { render: renderIncentiveTableOverview });
```

Place the registration call in the "Register Overview Renderers" section at the
bottom of the overview block, alongside the existing calls.

That's it. The dispatcher will match messages with `functionSet ===
'IncentiveTableData'` and call your function automatically.

---

## Step-by-step: key-value renderer

For function sets that contain a single object rather than a list (e.g. device
info, diagnostic state), use `overviewKV`:

```js
function renderPowerSequenceStateOverview(cmds) {
    var pairs = [];
    for (var i = 0; i < cmds.length; i++) {
        var ps = cmds[i].powerSequenceStateData;
        if (!ps) continue;
        if (ps.state)    pairs.push({label: 'State',    value: String(ps.state)});
        if (ps.activeSlotNumber !== undefined)
                         pairs.push({label: 'Active Slot', value: String(ps.activeSlotNumber)});
        if (ps.elapsedSlotTime)
                         pairs.push({label: 'Elapsed',  value: String(ps.elapsedSlotTime)});
    }
    return overviewKV(pairs);
}

registerOverview('PowerSequenceStateData', { render: renderPowerSequenceStateOverview });
```

---

## Using description context

Some renderers need to join description data (measurement descriptions, limit
descriptions, etc.) to display human-readable labels. Set `needsDescs: true` in
the registration and use the second argument:

```js
function renderMyDataOverview(cmds, descs) {
    // descs has the shape: { measurements: { "id": {...} }, limits: { "id": {...} } }
    var desc = (descs && descs.measurements && descs.measurements[id]) || {};
    var unit = desc.unit || '';
    // ...
}

registerOverview('MyListData', { needsDescs: true, render: renderMyDataOverview });
```

When `needsDescs` is true, the dispatcher calls `getDescriptions(traceId)` and
passes the result. The descriptions are cached per trace so the fetch only
happens once.

---

## Pattern-based registration

When multiple function set names share a common substring (e.g. all subscription
variants: `NodeManagementSubscriptionData`, `NodeManagementSubscriptionRequestCall`,
`NodeManagementSubscriptionDeleteCall`), use `registerOverviewPattern`:

```js
registerOverviewPattern(
    function(fs) { return fs.indexOf('Subscription') !== -1; },
    { render: renderSubscriptionOverview }
);
```

Pattern entries are checked **only if no exact match** exists. They are tried in
registration order, and the first match wins.

---

## `overviewTable` options

`overviewTable(title, columns, rows, opts)` accepts an optional fourth argument
with two maps:

### `cellClass` -- dynamic CSS class per cell

A map of column name to a function `(row) -> className`:

```js
overviewTable('Limits', ['ID', 'Value', 'Active'], rows, {
    cellClass: {
        'Active': function(row) {
            return row.Active ? 'enriched-active' : 'enriched-inactive';
        }
    }
});
```

### `cellFormat` -- custom display value per cell

A map of column name to a function `(row) -> displayString`. Without this, the
row value is converted to a string via `String()`. Use it when the stored value
differs from what should be displayed (e.g. boolean to checkmark):

```js
overviewTable('Limits', ['ID', 'Value', 'Active'], rows, {
    cellFormat: {
        'Active': function(row) { return row.Active ? '\u2713' : '\u2717'; }
    },
    cellClass: {
        'Active': function(row) {
            return row.Active ? 'enriched-active' : 'enriched-inactive';
        }
    }
});
```

Both options can be combined freely. Columns without entries in these maps render
normally.

---

## `fmtScaled` helper

SPINE uses `ScaledNumber` objects (`{number, scale}`) for values. The
`fmtScaled` helper converts them for display:

```js
fmtScaled({number: 29, scale: 0})          // "29"
fmtScaled({number: 29, scale: 0}, 'A')     // "29 A"
fmtScaled(null)                             // "-"
fmtScaled(undefined, 'W')                  // "-"
```

---

## How dispatch works

`renderOverviewTab` runs this logic for every SPINE data message:

1. **SHIP non-data** -- rendered by `renderShipOverview` (not in the registry).
2. **Result data** -- rendered by `renderResultOverview` (not in the registry).
3. **Exact registry match** -- `overviewRegistry[functionSet]`.
4. **Pattern match** -- first entry in `overviewPatterns` where `match(fs)` is
   truthy.
5. **No match** -- only the standard overview header is shown.

The render function return value is an HTML string (or empty string to show
nothing beyond the header).

---

## Existing renderers

For reference, these function sets are already registered:

| Function Set | Renderer | Type |
|---|---|---|
| `LoadControlLimitListData` | `renderLoadControlOverview` | table, needsDescs |
| `MeasurementListData` | `renderMeasurementOverview` | table, needsDescs |
| `SetpointListData` | `renderSetpointOverview` | table |
| `DeviceDiagnosisHeartbeatData` | `renderHeartbeatOverview` | KV |
| `DeviceDiagnosisStateData` | `renderDiagnosisStateOverview` | KV |
| `DeviceClassificationManufacturerData` | `renderManufacturerOverview` | KV |
| `MeasurementDescriptionListData` | `renderMeasurementDescOverview` | table |
| `LoadControlLimitDescriptionListData` | `renderLimitDescOverview` | table |
| `ElectricalConnectionParameterDescriptionListData` | `renderElecParamDescOverview` | table |
| `SetpointDescriptionListData` | `renderSetpointDescOverview` | table |
| `NodeManagementDetailedDiscoveryData` | `renderDiscoveryOverview` | custom |
| `NodeManagementUseCaseData` | `renderUseCaseOverview` | custom |
| `*Subscription*` (pattern) | `renderSubscriptionOverview` | table |
| `*Binding*` (pattern) | `renderBindingOverview` | table |

---

## Checklist

When adding a new renderer:

- [ ] Write the render function (extract data from `cmds`, build rows/pairs)
- [ ] Call `registerOverview()` or `registerOverviewPattern()`
- [ ] Add a CHANGELOG.md entry

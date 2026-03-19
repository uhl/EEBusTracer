// EEBusTracer Web UI

(function() {
    'use strict';

    let ws = null;
    let selectedMsgId = null;
    let currentFilter = {};
    let currentMsg = null;
    let descriptionCache = {};
    let autoScroll = true;
    var orphanedIds = new Set();
    var useCaseContextMap = {};

    var PAGE_SIZE = 2000;
    var currentOffset = 0;
    var lastTotalCount = typeof window.TOTAL_MESSAGES !== 'undefined' ? window.TOTAL_MESSAGES : 0;

    // --- Capture Mode Selector ---
    const captureMode = document.getElementById('capture-mode');
    const udpInputs = document.getElementById('udp-inputs');
    const tcpInputs = document.getElementById('tcp-inputs');
    const logtailInputs = document.getElementById('logtail-inputs');

    if (captureMode) {
        captureMode.addEventListener('change', function() {
            if (udpInputs) udpInputs.style.display = this.value === 'udp' ? '' : 'none';
            if (tcpInputs) tcpInputs.style.display = this.value === 'tcp' ? '' : 'none';
            if (logtailInputs) logtailInputs.style.display = this.value === 'logtail' ? '' : 'none';
            populateRecentTargets();
        });
    }

    // --- Recent Targets Cache ---
    var RECENT_TARGETS_KEY = 'eebustracer-recent-targets';
    var RECENT_TARGETS_MAX = 10;

    function loadRecentTargets() {
        try {
            var data = localStorage.getItem(RECENT_TARGETS_KEY);
            if (!data) return [];
            var entries = JSON.parse(data);
            if (!Array.isArray(entries)) return [];
            return entries.slice(0, RECENT_TARGETS_MAX);
        } catch (e) {
            return [];
        }
    }

    function saveRecentTarget(entry) {
        var entries = loadRecentTargets();
        // Deduplicate by mode+host+port or mode+path
        entries = entries.filter(function(e) {
            if (e.mode !== entry.mode) return true;
            if (entry.path) return e.path !== entry.path;
            return !(e.host === entry.host && e.port === entry.port);
        });
        entry.ts = Date.now();
        entries.unshift(entry);
        entries = entries.slice(0, RECENT_TARGETS_MAX);
        try {
            localStorage.setItem(RECENT_TARGETS_KEY, JSON.stringify(entries));
        } catch (e) {
            // localStorage full or unavailable — ignore
        }
    }

    function populateRecentTargets() {
        var entries = loadRecentTargets();
        var mode = captureMode ? captureMode.value : 'udp';

        var udpList = document.getElementById('recent-hosts-udp');
        var tcpList = document.getElementById('recent-hosts-tcp');
        var pathList = document.getElementById('recent-paths');

        if (udpList) {
            udpList.innerHTML = '';
            entries.filter(function(e) { return e.mode === 'udp'; }).forEach(function(e) {
                var opt = document.createElement('option');
                opt.value = e.host;
                opt.label = e.host + ':' + e.port;
                udpList.appendChild(opt);
            });
        }
        if (tcpList) {
            tcpList.innerHTML = '';
            entries.filter(function(e) { return e.mode === 'tcp'; }).forEach(function(e) {
                var opt = document.createElement('option');
                opt.value = e.host;
                opt.label = e.host + ':' + e.port;
                tcpList.appendChild(opt);
            });
        }
        if (pathList) {
            pathList.innerHTML = '';
            entries.filter(function(e) { return e.mode === 'logtail'; }).forEach(function(e) {
                var opt = document.createElement('option');
                opt.value = e.path;
                pathList.appendChild(opt);
            });
        }
    }

    // Auto-fill port when user selects a host from the datalist
    function setupHostAutoFill(hostInput, portInput, mode) {
        if (!hostInput || !portInput) return;
        hostInput.addEventListener('input', function() {
            var entries = loadRecentTargets();
            var host = hostInput.value.trim();
            for (var i = 0; i < entries.length; i++) {
                if (entries[i].mode === mode && entries[i].host === host) {
                    portInput.value = entries[i].port;
                    break;
                }
            }
        });
    }

    // --- Capture Controls ---
    const btnStart = document.getElementById('btn-start-capture');
    const btnStop = document.getElementById('btn-stop-capture');
    const statusEl = document.getElementById('capture-status');
    const hostInput = document.getElementById('target-host');
    const portInput = document.getElementById('target-port');
    const tcpHostInput = document.getElementById('tcp-host');
    const tcpPortInput = document.getElementById('tcp-port');
    const logFileInput = document.getElementById('log-file-path');

    setupHostAutoFill(hostInput, portInput, 'udp');
    setupHostAutoFill(tcpHostInput, tcpPortInput, 'tcp');
    populateRecentTargets();

    if (btnStart) {
        btnStart.addEventListener('click', async () => {
            const mode = captureMode ? captureMode.value : 'udp';

            try {
                let resp;
                if (mode === 'logtail') {
                    const path = logFileInput ? logFileInput.value.trim() : '';
                    if (!path) {
                        alert('Enter the log file path');
                        return;
                    }
                    resp = await fetch('/api/capture/start/logtail', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({path: path})
                    });
                } else if (mode === 'tcp') {
                    const host = tcpHostInput ? tcpHostInput.value.trim() : '';
                    const port = parseInt(tcpPortInput ? tcpPortInput.value : '') || 54546;
                    if (!host) {
                        alert('Enter the TCP log server IP address');
                        return;
                    }
                    resp = await fetch('/api/capture/start/tcp', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({host: host, port: port})
                    });
                } else {
                    const host = hostInput ? hostInput.value.trim() : '';
                    const port = parseInt(portInput.value) || 4712;
                    if (!host) {
                        alert('Enter the EEBus stack IP address');
                        return;
                    }
                    resp = await fetch('/api/capture/start', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({host: host, port: port})
                    });
                }

                if (resp.ok) {
                    const data = await resp.json();
                    // Save target to recent cache
                    if (mode === 'logtail') {
                        saveRecentTarget({mode: 'logtail', path: logFileInput.value.trim()});
                    } else if (mode === 'tcp') {
                        saveRecentTarget({mode: 'tcp', host: tcpHostInput.value.trim(), port: parseInt(tcpPortInput.value) || 54546});
                    } else {
                        saveRecentTarget({mode: 'udp', host: hostInput.value.trim(), port: parseInt(portInput.value) || 4712});
                    }
                    btnStart.disabled = true;
                    btnStop.disabled = false;
                    statusEl.textContent = 'Recording...';
                    connectWebSocket(data.traceId);
                    window.location.href = '/traces/' + data.traceId;
                } else {
                    const err = await resp.json();
                    alert('Start failed: ' + err.error);
                }
            } catch (e) {
                alert('Start failed: ' + e.message);
            }
        });
    }

    if (btnStop) {
        btnStop.addEventListener('click', async () => {
            try {
                const resp = await fetch('/api/capture/stop', {method: 'POST'});
                if (resp.ok) {
                    btnStart.disabled = false;
                    btnStop.disabled = true;
                    statusEl.textContent = 'Idle';
                    if (ws) { ws.close(); ws = null; }
                    // Re-compact capture controls on trace pages
                    if (typeof window.TRACE_ID !== 'undefined') {
                        setCaptureCompact(true);
                    }
                }
            } catch (e) {
                alert('Stop failed: ' + e.message);
            }
        });
    }

    // --- Live Filter Matching ---
    // Check if a WebSocket message matches the currently active filters.
    function matchesFilter(msg) {
        var cmdEl = document.getElementById('filter-cmd');
        if (cmdEl && cmdEl.value && (msg.cmdClassifier || '') !== cmdEl.value) return false;

        var searchEl = document.getElementById('filter-search');
        if (searchEl && searchEl.value.trim()) {
            var term = searchEl.value.trim().toLowerCase();
            var haystack = [
                msg.shipMsgType, msg.cmdClassifier, msg.functionSet,
                msg.deviceSource, msg.deviceDest, msg.direction,
                msg.msgCounter
            ].join(' ').toLowerCase();
            if (haystack.indexOf(term) === -1) return false;
        }

        return true;
    }

    // --- WebSocket ---
    function connectWebSocket(traceId) {
        if (ws) { ws.close(); }
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(proto + '//' + location.host + '/api/traces/' + traceId + '/live');

        ws.onmessage = function(event) {
            const data = JSON.parse(event.data);
            if (data.type === 'message') {
                if (matchesFilter(data.payload)) {
                    appendMessageRow(data.payload);
                }
                updateStatusBar();
            } else if (data.type === 'mdns_device') {
                // Handle mDNS device event if discovery panel is present
                if (typeof updateDiscoveryPanel === 'function') {
                    updateDiscoveryPanel(data.payload);
                }
            }
        };

        ws.onclose = function() { ws = null; };
    }

    // --- Compact capture controls on trace pages ---
    var captureControlsEl = document.querySelector('.capture-controls');
    var captureExpandBtn = document.getElementById('capture-expand-btn');

    function setCaptureCompact(compact) {
        if (!captureControlsEl) return;
        if (compact) {
            captureControlsEl.classList.add('capture-compact');
        } else {
            captureControlsEl.classList.remove('capture-compact');
        }
    }

    if (captureExpandBtn) {
        captureExpandBtn.addEventListener('click', function() {
            setCaptureCompact(false);
        });
    }

    // Auto-connect if on a trace page and capturing
    if (typeof window.TRACE_ID !== 'undefined') {
        fetch('/api/capture/status')
            .then(r => r.json())
            .then(data => {
                if (data.capturing && data.traceId === window.TRACE_ID) {
                    connectWebSocket(window.TRACE_ID);
                    setCaptureCompact(false);
                    // Reflect source type in mode selector
                    if (captureMode && data.sourceType) {
                        if (data.sourceType === 'logtail') {
                            captureMode.value = 'logtail';
                        } else if (data.sourceType === 'tcp') {
                            captureMode.value = 'tcp';
                        } else {
                            captureMode.value = 'udp';
                        }
                        captureMode.dispatchEvent(new Event('change'));
                    }
                } else {
                    // Not capturing on this trace — collapse controls
                    setCaptureCompact(true);
                }
            })
            .catch(function() {
                setCaptureCompact(true);
            });

        // Load sidebar data
        loadBookmarks();
        loadDevicePanel();
        loadPresets();

        // Load orphaned request IDs
        fetch('/api/traces/' + window.TRACE_ID + '/orphaned-requests')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                orphanedIds = new Set(data.ids || []);
                // Mark existing rows
                document.querySelectorAll('.msg-row').forEach(function(row) {
                    var id = parseInt(row.dataset.id);
                    if (orphanedIds.has(id)) row.classList.add('msg-orphan');
                });
            })
            .catch(function() {});

        // Load use case context
        fetch('/api/traces/' + window.TRACE_ID + '/usecase-context')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                populateUseCaseDropdown(data);
            })
            .catch(function() {});

    }

    // --- Message Table ---
    function appendMessageRow(msg) {
        const tbody = document.getElementById('message-tbody');
        if (!tbody) return;

        const tr = document.createElement('tr');
        tr.className = 'msg-row' + (orphanedIds.has(msg.id) ? ' msg-orphan' : '');
        tr.dataset.id = msg.id;
        tr.onclick = function() { showDetail(msg.traceId, msg.id); };

        const ts = new Date(msg.timestamp);
        const timeStr = ts.toTimeString().substring(0, 8) + '.' + String(ts.getMilliseconds()).padStart(3, '0');

        var arrow = msg.direction === 'incoming' ? '\u2190' : '\u2192';
        var cls = msg.cmdClassifier || '';
        tr.innerHTML =
            '<td class="col-bookmark" id="bm-' + msg.id + '"></td>' +
            '<td class="col-seq">' + (msg.sequenceNum || '') + '</td>' +
            '<td>' + timeStr + '</td>' +
            '<td class="col-comm">' + (msg.deviceSource || '') + ' <span class="dir-arrow dir-' + msg.direction + '">' + arrow + '</span> ' + (msg.deviceDest || '') + '</td>' +
            '<td class="cmd-' + cls + '">' + cls + '</td>' +
            '<td>' + (msg.functionSet || msg.shipMsgType || '') + '</td>';

        tbody.appendChild(tr);

        // Animate new row entrance
        tr.classList.add('msg-row-enter');
        setTimeout(function() { tr.classList.remove('msg-row-enter'); }, 400);

        if (autoScroll) {
            const container = tbody.closest('.message-table-container');
            if (container) {
                container.scrollTop = container.scrollHeight;
            }
        } else {
            var btn = document.getElementById('auto-scroll-btn');
            if (btn) btn.classList.add('has-new');
        }
    }

    // --- Message Detail ---
    window.showDetail = async function(traceId, msgId) {
        document.querySelectorAll('.msg-row').forEach(r => r.classList.remove('selected'));
        const row = document.querySelector('.msg-row[data-id="' + msgId + '"]');
        if (row) row.classList.add('selected');
        selectedMsgId = msgId;

        // Clear previous correlation highlights
        document.querySelectorAll('.msg-row.msg-correlated').forEach(r => r.classList.remove('msg-correlated'));

        // Fetch related and highlight correlated rows
        fetch('/api/traces/' + traceId + '/messages/' + msgId + '/related')
            .then(function(r) { return r.json(); })
            .then(function(related) {
                for (var i = 0; i < related.length; i++) {
                    var relRow = document.querySelector('.msg-row[data-id="' + related[i].message.id + '"]');
                    if (relRow && relRow !== row) relRow.classList.add('msg-correlated');
                }
            })
            .catch(function() {});

        try {
            const resp = await fetch('/api/traces/' + traceId + '/messages/' + msgId);
            const msg = await resp.json();
            currentMsg = msg;
            const content = document.getElementById('detail-content');

            content.innerHTML = '<div class="empty-state">Loading...</div>';

            // Show detail actions
            const actions = document.getElementById('detail-actions');
            if (actions) actions.style.display = 'flex';

            setupTabs(msg);
            // Reset active tab highlight to Overview
            document.querySelectorAll('.detail-panel .tab').forEach(t => t.classList.remove('active'));
            var overviewTab = document.querySelector('.detail-panel .tab[data-tab="overview"]');
            if (overviewTab) overviewTab.classList.add('active');
            renderOverviewTab(msg, content);
        } catch (e) {
            console.error('Failed to load message detail:', e);
        }
    };

    // --- Description Context ---
    async function getDescriptions(traceId) {
        if (descriptionCache[traceId]) {
            return descriptionCache[traceId];
        }
        try {
            var resp = await fetch('/api/traces/' + traceId + '/descriptions');
            var data = await resp.json();
            descriptionCache[traceId] = data;
            return data;
        } catch (e) {
            return {measurements: {}, limits: {}};
        }
    }

    // --- Overview Tab ---

    function scaledNumberValue(sn) {
        if (!sn || sn.number === undefined || sn.number === null) return null;
        var scale = sn.scale || 0;
        return sn.number * Math.pow(10, scale);
    }

    function scopeDisplay(scope) {
        switch(scope) {
            case 'overloadProtection': return 'Overload Protection';
            case 'selfConsumption': return 'Self Consumption';
            case 'discharge': return 'Discharge';
            default: return scope;
        }
    }

function categoryAbbr(cat) {
        switch(cat) {
            case 'obligation': return 'ob.';
            case 'recommendation': return 'rec.';
            default: return cat;
        }
    }

    function typeDisplay(mtype) {
        switch(mtype) {
            case 'current': return 'Current';
            case 'power': return 'Power';
            case 'energy': return 'Energy';
            case 'voltage': return 'Voltage';
            default: return mtype;
        }
    }

    function extractCmdsJS(payload) {
        if (!payload) return [];
        try {
            var cmds = payload.datagram.payload.cmd;
            if (!cmds) return [];
            if (!Array.isArray(cmds)) return [cmds];
            return cmds;
        } catch(e) {
            return [];
        }
    }

    function hasResultData(cmds) {
        for (var i = 0; i < cmds.length; i++) {
            if (cmds[i].resultData) return true;
        }
        return false;
    }

    function shortenDevice(addr) {
        if (!addr) return '';
        // d:_i:37916_CEM-400000270 → CEM-400000270
        var parts = addr.split('_');
        if (parts.length > 1) return parts[parts.length - 1];
        return addr;
    }

    function overviewHeader(msg) {
        var dir = msg.direction || '';
        var arrow = dir === 'incoming' ? '\u2190 IN' : '\u2192 OUT';
        var arrowClass = dir === 'incoming' ? 'dir-incoming' : 'dir-outgoing';
        var classifier = msg.cmdClassifier || '';
        var fs = msg.functionSet || '';
        var src = shortenDevice(msg.deviceSource);
        var dst = shortenDevice(msg.deviceDest);
        var ctr = msg.msgCounter || '';
        var ref = msg.msgCounterRef || '';

        var html = '<div class="overview-summary">';
        html += '<div class="overview-kv"><span class="overview-label">Direction</span><span class="' + arrowClass + '">' + escapeHtml(arrow) + '</span></div>';
        if (classifier) {
            var badgeCls = 'overview-badge-cmd overview-badge-' + classifier;
            html += '<div class="overview-kv"><span class="overview-label">Classifier</span><span class="' + badgeCls + '">' + escapeHtml(classifier) + '</span></div>';
        }
        if (fs) html += '<div class="overview-kv"><span class="overview-label">Function</span><span>' + escapeHtml(fs) + '</span></div>';
        if (src || dst) html += '<div class="overview-kv"><span class="overview-label">Path</span><span>' + escapeHtml(src) + ' \u2192 ' + escapeHtml(dst) + '</span></div>';
        if (ctr) html += '<div class="overview-kv"><span class="overview-label">MsgCounter</span><span>' + escapeHtml(ctr) + '</span></div>';
        if (ref) html += '<div class="overview-kv"><span class="overview-label">MsgCounterRef</span><span>' + escapeHtml(ref) + '</span></div>';
        html += '</div>';
        return html;
    }

    // --- Overview Registry ---
    var overviewRegistry = {};
    var overviewPatterns = [];

    function registerOverview(functionSet, opts) {
        overviewRegistry[functionSet] = {
            needsDescs: opts.needsDescs || false,
            render: opts.render
        };
    }

    function registerOverviewPattern(matchFn, opts) {
        overviewPatterns.push({
            match: matchFn,
            needsDescs: opts.needsDescs || false,
            render: opts.render
        });
    }

    function fmtScaled(sn, unit) {
        var val = scaledNumberValue(sn);
        if (val === null) return '-';
        return val + (unit ? ' ' + unit : '');
    }

    function overviewTable(title, columns, rows, opts) {
        if (rows.length === 0) return '';
        opts = opts || {};
        var cellClass = opts.cellClass || {};
        var cellFormat = opts.cellFormat || {};
        var html = '<div class="enriched-header">' + escapeHtml(title) + '</div>';
        html += '<table class="enriched-table"><thead><tr>';
        for (var c = 0; c < columns.length; c++) {
            html += '<th>' + escapeHtml(columns[c]) + '</th>';
        }
        html += '</tr></thead><tbody>';
        for (var r = 0; r < rows.length; r++) {
            var row = rows[r];
            html += '<tr>';
            for (var c2 = 0; c2 < columns.length; c2++) {
                var col = columns[c2];
                var val;
                if (cellFormat[col]) {
                    val = cellFormat[col](row);
                } else {
                    val = row[col] !== undefined && row[col] !== null ? String(row[col]) : '';
                }
                var cls = cellClass[col] ? cellClass[col](row) : '';
                if (cls) {
                    html += '<td class="' + cls + '">' + escapeHtml(val) + '</td>';
                } else {
                    html += '<td>' + escapeHtml(val) + '</td>';
                }
            }
            html += '</tr>';
        }
        html += '</tbody></table>';
        return html;
    }

    function overviewKV(pairs) {
        var html = '<div class="overview-summary">';
        for (var i = 0; i < pairs.length; i++) {
            var p = pairs[i];
            html += '<div class="overview-kv"><span class="overview-label">' + escapeHtml(p.label) + '</span><span>' + escapeHtml(String(p.value)) + '</span></div>';
        }
        html += '</div>';
        return html;
    }

    async function renderOverviewTab(msg, container) {
        var shipType = msg.shipMsgType || '';
        var fs = msg.functionSet || '';
        var orphanWarning = orphanedIds.has(msg.id) ? '<div class="overview-warning">No response received for this request</div>' : '';

        if (shipType && shipType !== 'data') {
            container.innerHTML = orphanWarning + renderShipOverview(msg);
            return;
        }

        var payload = msg.spinePayload;
        if (typeof payload === 'string') {
            try { payload = JSON.parse(payload); } catch(e) { payload = null; }
        }
        var cmds = extractCmdsJS(payload);

        if (hasResultData(cmds)) {
            container.innerHTML = orphanWarning + overviewHeader(msg) + renderResultOverview(cmds, msg);
            return;
        }

        // Registry lookup: exact match first, then pattern match
        var entry = overviewRegistry[fs];
        if (!entry) {
            for (var i = 0; i < overviewPatterns.length; i++) {
                if (overviewPatterns[i].match(fs)) { entry = overviewPatterns[i]; break; }
            }
        }

        var descs = null;
        if (entry && entry.needsDescs) {
            descs = await getDescriptions(msg.traceId);
        }

        var body = entry ? entry.render(cmds, descs) : '';
        container.innerHTML = orphanWarning + overviewHeader(msg) + (body || '');
    }

    function shipTypeBadge(type) {
        var colors = {
            init:                      { bg: 'rgba(120,113,108,0.12)', fg: '#78716c' },
            connectionHello:           { bg: 'rgba(37,99,235,0.10)',   fg: '#2563eb' },
            messageProtocolHandshake:  { bg: 'rgba(147,51,234,0.10)',  fg: '#9333ea' },
            connectionPinState:        { bg: 'rgba(234,88,12,0.10)',   fg: '#ea580c' },
            accessMethods:             { bg: 'rgba(13,148,136,0.10)',  fg: '#0d9488' },
            connectionClose:           { bg: 'rgba(220,38,38,0.10)',   fg: '#dc2626' }
        };
        var c = colors[type] || colors.init;
        return '<span class="overview-badge-cmd" style="background:' + c.bg + ';color:' + c.fg + '">' + escapeHtml(type) + '</span>';
    }

    function kvRow(label, valueHtml) {
        return '<div class="overview-kv"><span class="overview-label">' + escapeHtml(label) + '</span><span>' + valueHtml + '</span></div>';
    }

    function statusIndicator(text, color) {
        return '<span style="display:inline-flex;align-items:center;gap:5px">' +
            '<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:' + color + '"></span>' +
            escapeHtml(text) + '</span>';
    }

    function formatShipVersion(ver) {
        if (!ver) return '';
        if (typeof ver === 'object' && ver.major !== undefined) {
            return String(ver.major) + '.' + String(ver.minor || 0);
        }
        return String(ver);
    }

    function formatShipFormats(fmts) {
        if (!fmts) return '';
        if (Array.isArray(fmts)) {
            var parts = [];
            for (var i = 0; i < fmts.length; i++) {
                var f = fmts[i];
                parts.push(typeof f === 'string' ? f : JSON.stringify(f));
            }
            return parts.join(', ');
        }
        return String(fmts);
    }

    function renderShipOverview(msg) {
        var shipType = msg.shipMsgType || '';
        var payload = msg.normalizedJson || msg.shipPayload || '{}';
        if (typeof payload === 'string') {
            try { payload = JSON.parse(payload); } catch(e) { payload = {}; }
        }

        var html = '';

        // Header section: direction + SHIP type badge + path
        html += '<div class="overview-summary">';
        var dir = msg.direction || '';
        var arrow = dir === 'incoming' ? '\u2190 IN' : '\u2192 OUT';
        var arrowClass = dir === 'incoming' ? 'dir-incoming' : 'dir-outgoing';
        html += kvRow('Direction', '<span class="' + arrowClass + '">' + escapeHtml(arrow) + '</span>');
        html += kvRow('SHIP Type', shipTypeBadge(shipType));
        var src = shortenDevice(msg.deviceSource);
        var dst = shortenDevice(msg.deviceDest);
        if (src || dst) html += kvRow('Path', escapeHtml(src) + ' \u2192 ' + escapeHtml(dst));
        html += '</div>';

        // Per-type detail section
        html += '<div class="overview-summary">';

        if (shipType === 'init') {
            html += kvRow('Description', escapeHtml('SHIP Init frame (CMI header byte 0x00)'));

        } else if (shipType === 'connectionHello') {
            var hello = payload.connectionHello || payload;
            if (hello.phase) {
                var phaseColors = { pending: '#d97706', ready: '#16a34a', aborted: '#dc2626' };
                var phaseColor = phaseColors[hello.phase] || '#78716c';
                html += kvRow('Phase', statusIndicator(String(hello.phase), phaseColor));
            }
            if (hello.waiting !== undefined) {
                html += kvRow('Waiting', escapeHtml(String(hello.waiting)) + ' ms');
            }
            if (hello.prolongationRequest !== undefined) {
                html += kvRow('Prolongation', escapeHtml(String(hello.prolongationRequest)));
            }

        } else if (shipType === 'messageProtocolHandshake') {
            var hs = payload.messageProtocolHandshake || payload;
            if (hs.handshakeType) {
                var hsBg = hs.handshakeType === 'announceMax'
                    ? 'rgba(37,99,235,0.10)' : 'rgba(22,163,74,0.10)';
                var hsFg = hs.handshakeType === 'announceMax'
                    ? '#2563eb' : '#16a34a';
                html += kvRow('Handshake', '<span class="overview-badge-cmd" style="background:' + hsBg + ';color:' + hsFg + '">' + escapeHtml(String(hs.handshakeType)) + '</span>');
            }
            if (hs.version) {
                html += kvRow('Version', escapeHtml(formatShipVersion(hs.version)));
            }
            if (hs.formats) {
                html += kvRow('Formats', escapeHtml(formatShipFormats(hs.formats)));
            }

        } else if (shipType === 'connectionPinState') {
            var pin = payload.connectionPinState || payload;
            if (pin.pinState) {
                var pinColors = { none: '#16a34a', pinOk: '#16a34a', required: '#d97706', optional: '#2563eb' };
                var pinColor = pinColors[pin.pinState] || '#78716c';
                html += kvRow('Pin State', statusIndicator(String(pin.pinState), pinColor));
            }
            if (pin.inputPermission) {
                html += kvRow('Input Permission', escapeHtml(String(pin.inputPermission)));
            }

        } else if (shipType === 'accessMethods') {
            // Detect request vs response by top-level key
            var isRequest = !!payload.accessMethodsRequest;
            var isResponse = !isRequest;
            var amBadgeBg = isRequest ? 'rgba(37,99,235,0.10)' : 'rgba(22,163,74,0.10)';
            var amBadgeFg = isRequest ? '#2563eb' : '#16a34a';
            var amLabel = isRequest ? 'Request' : 'Response';
            html += kvRow('Type', '<span class="overview-badge-cmd" style="background:' + amBadgeBg + ';color:' + amBadgeFg + '">' + escapeHtml(amLabel) + '</span>');
            var am = payload.accessMethodsRequest || payload.accessMethods || payload;
            if (am.id) html += kvRow('ID', escapeHtml(String(am.id)));
            // Show dns.uri if present
            var dns = am.dns;
            if (dns && dns.uri) {
                html += kvRow('DNS URI', escapeHtml(String(dns.uri)));
            }

        } else if (shipType === 'connectionClose') {
            var cc = payload.connectionClose || payload;
            if (cc.phase) {
                var ccColors = { announce: '#d97706', confirm: '#dc2626' };
                var ccColor = ccColors[cc.phase] || '#78716c';
                html += kvRow('Phase', statusIndicator(String(cc.phase), ccColor));
            }
            if (cc.reason) html += kvRow('Reason', escapeHtml(String(cc.reason)));
            if (cc.maxTime !== undefined) html += kvRow('Max Time', escapeHtml(String(cc.maxTime)) + ' ms');
        }

        html += '</div>';
        return html;
    }

    function renderResultOverview(cmds, msg) {
        var html = '<div class="overview-result">';
        for (var i = 0; i < cmds.length; i++) {
            var rd = cmds[i].resultData;
            if (!rd) continue;
            var errNum = rd.errorNumber !== undefined ? rd.errorNumber : 0;
            if (errNum === 0) {
                html += '<span class="overview-badge-accepted">\u2713 Accepted</span>';
            } else {
                html += '<span class="overview-badge-rejected">\u2717 Rejected \u2014 Error #' + escapeHtml(String(errNum)) + '</span>';
                if (rd.description) {
                    html += ' <span>' + escapeHtml(String(rd.description)) + '</span>';
                }
            }
        }
        html += '</div>';
        return html;
    }

    function renderLoadControlOverview(cmds, descs) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.loadControlLimitListData) continue;
            var data = cmd.loadControlLimitListData.loadControlLimitData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                if (d.limitId === undefined || d.limitId === null) continue;
                var id = String(d.limitId);
                var desc = (descs && descs.limits && descs.limits[id]) || {};
                var scopeRaw = desc.scopeType || '';
                var scope = scopeRaw ? scopeDisplay(scopeRaw) : '';
                if (desc.limitCategory) scope += ' (' + categoryAbbr(desc.limitCategory) + ')';
                rows.push({
                    ID: id,
                    Scope: scope,
                    Phase: desc.phase || '',
                    Value: fmtScaled(d.value, desc.unit || ''),
                    Active: d.isLimitActive === true
                });
            }
        }
        return overviewTable('Load Control Limits', ['ID', 'Scope', 'Phase', 'Value', 'Active'], rows, {
            cellClass: {'Active': function(row) { return row.Active ? 'enriched-active' : 'enriched-inactive'; }},
            cellFormat: {'Active': function(row) { return row.Active ? '\u2713' : '\u2717'; }}
        });
    }

    function renderMeasurementOverview(cmds, descs) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.measurementListData) continue;
            var data = cmd.measurementListData.measurementData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                if (d.measurementId === undefined || d.measurementId === null) continue;
                var id = String(d.measurementId);
                var desc = (descs && descs.measurements && descs.measurements[id]) || {};
                rows.push({
                    ID: id,
                    Type: desc.measurementType ? typeDisplay(desc.measurementType) : '',
                    Phase: desc.phase || '',
                    Value: fmtScaled(d.value, desc.unit || '')
                });
            }
        }
        return overviewTable('Measurements', ['ID', 'Type', 'Phase', 'Value'], rows);
    }

    function renderSetpointOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.setpointListData) continue;
            var data = cmd.setpointListData.setpointData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                if (d.setpointId === undefined || d.setpointId === null) continue;
                rows.push({ID: String(d.setpointId), Value: fmtScaled(d.value)});
            }
        }
        return overviewTable('Setpoints', ['ID', 'Value'], rows);
    }

    function renderHeartbeatOverview(cmds) {
        var pairs = [];
        for (var i = 0; i < cmds.length; i++) {
            var hb = cmds[i].deviceDiagnosisHeartbeatData;
            if (!hb) continue;
            if (hb.heartbeatCounter !== undefined) pairs.push({label: 'Counter', value: String(hb.heartbeatCounter)});
            if (hb.heartbeatTimeout) pairs.push({label: 'Timeout', value: String(hb.heartbeatTimeout)});
        }
        return overviewKV(pairs);
    }

    function renderDiagnosisStateOverview(cmds) {
        var pairs = [];
        for (var i = 0; i < cmds.length; i++) {
            var ds = cmds[i].deviceDiagnosisStateData;
            if (!ds) continue;
            if (ds.operatingState) pairs.push({label: 'Operating State', value: String(ds.operatingState)});
            if (ds.powerSupplyCondition) pairs.push({label: 'Power Supply', value: String(ds.powerSupplyCondition)});
        }
        return overviewKV(pairs);
    }

    function renderManufacturerOverview(cmds) {
        var pairs = [];
        for (var i = 0; i < cmds.length; i++) {
            var mf = cmds[i].deviceClassificationManufacturerData;
            if (!mf) continue;
            if (mf.brandName) pairs.push({label: 'Brand', value: String(mf.brandName)});
            if (mf.deviceName) pairs.push({label: 'Model', value: String(mf.deviceName)});
            if (mf.serialNumber) pairs.push({label: 'Serial', value: String(mf.serialNumber)});
            if (mf.softwareRevision) pairs.push({label: 'Software', value: String(mf.softwareRevision)});
            if (mf.hardwareRevision) pairs.push({label: 'Hardware', value: String(mf.hardwareRevision)});
        }
        return overviewKV(pairs);
    }

    function renderMeasurementDescOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.measurementDescriptionListData) continue;
            var data = cmd.measurementDescriptionListData.measurementDescriptionData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                rows.push({
                    ID: d.measurementId !== undefined ? String(d.measurementId) : '-',
                    Type: d.measurementType || '',
                    Unit: d.unit || '',
                    Scope: d.scopeType || ''
                });
            }
        }
        return overviewTable('Measurement Descriptions', ['ID', 'Type', 'Unit', 'Scope'], rows);
    }

    function renderLimitDescOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.loadControlLimitDescriptionListData) continue;
            var data = cmd.loadControlLimitDescriptionListData.loadControlLimitDescriptionData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                rows.push({
                    LimitID: d.limitId !== undefined ? String(d.limitId) : '-',
                    MeasID: d.measurementId !== undefined ? String(d.measurementId) : '-',
                    Scope: d.scopeType || '',
                    Category: d.limitCategory || '',
                    Unit: d.unit || ''
                });
            }
        }
        return overviewTable('Limit Descriptions', ['LimitID', 'MeasID', 'Scope', 'Category', 'Unit'], rows);
    }

    function renderElecParamDescOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.electricalConnectionParameterDescriptionListData) continue;
            var data = cmd.electricalConnectionParameterDescriptionListData.electricalConnectionParameterDescriptionData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                rows.push({
                    ParamID: d.parameterId !== undefined ? String(d.parameterId) : '-',
                    MeasID: d.measurementId !== undefined ? String(d.measurementId) : '-',
                    Phase: d.acMeasuredPhases || ''
                });
            }
        }
        return overviewTable('Electrical Connection Parameters', ['ParamID', 'MeasID', 'Phase'], rows);
    }

    function renderSetpointDescOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            if (!cmd.setpointDescriptionListData) continue;
            var data = cmd.setpointDescriptionListData.setpointDescriptionData || [];
            for (var j = 0; j < data.length; j++) {
                var d = data[j];
                rows.push({
                    ID: d.setpointId !== undefined ? String(d.setpointId) : '-',
                    Type: d.setpointType || '',
                    Unit: d.unit || ''
                });
            }
        }
        return overviewTable('Setpoint Descriptions', ['ID', 'Type', 'Unit'], rows);
    }

    function renderDiscoveryOverview(cmds) {
        var html = '';
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            var dd = cmd.nodeManagementDetailedDiscoveryData;
            if (!dd) continue;

            // Device info
            if (dd.specificationVersionList) {
                html += '<div class="enriched-header">Specification Versions</div>';
                var versions = dd.specificationVersionList.specificationVersion || [];
                if (!Array.isArray(versions)) versions = [versions];
                for (var v = 0; v < versions.length; v++) {
                    html += '<span class="uc-pill uc-available">' + escapeHtml(String(versions[v])) + '</span> ';
                }
                html += '<br><br>';
            }

            // Entity/feature tree
            var entities = dd.entityInformation || [];
            if (!Array.isArray(entities)) entities = [entities];
            var features = dd.featureInformation || [];
            if (!Array.isArray(features)) features = [features];

            if (entities.length > 0) {
                html += '<div class="enriched-header">Entities & Features</div>';
                html += '<div class="overview-tree">';
                for (var e = 0; e < entities.length; e++) {
                    var ent = entities[e];
                    var desc = ent.description || {};
                    var addr = ent.address ? JSON.stringify(ent.address.entity || ent.address) : '';
                    var etype = desc.entityType || '';
                    html += '<div class="overview-tree-entity">';
                    html += '<strong>' + escapeHtml(etype || 'Entity') + '</strong>';
                    if (addr) html += ' <span class="feature-addr">[' + escapeHtml(addr) + ']</span>';
                    if (desc.description) html += ' \u2014 ' + escapeHtml(String(desc.description));

                    // Find features belonging to this entity
                    var entityAddr = ent.address ? (ent.address.entity || ent.address) : null;
                    for (var f = 0; f < features.length; f++) {
                        var feat = features[f];
                        var fdesc = feat.description || {};
                        var faddr = feat.address ? feat.address : {};
                        html += '<div class="overview-tree-feature">';
                        html += escapeHtml(fdesc.featureType || '?');
                        if (fdesc.role) html += ' <span class="feature-role">(' + escapeHtml(fdesc.role) + ')</span>';
                        if (faddr.feature !== undefined) html += ' <span class="feature-addr">[' + escapeHtml(String(faddr.feature)) + ']</span>';
                        html += '</div>';
                    }
                    html += '</div>';
                }
                html += '</div>';
            }
        }
        return html || '<div class="empty-state">No discovery data</div>';
    }

    function renderUseCaseOverview(cmds) {
        var html = '';
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            var ucd = cmd.nodeManagementUseCaseData;
            if (!ucd) continue;
            var entries = ucd.useCaseInformation || [];
            if (!Array.isArray(entries)) entries = [entries];
            for (var j = 0; j < entries.length; j++) {
                var entry = entries[j];
                var actor = entry.actor || '';
                html += '<div style="margin-bottom:8px;">';
                html += '<span class="intel-actor-badge">' + escapeHtml(actor) + '</span> ';
                var useCases = entry.useCaseSupport || [];
                if (!Array.isArray(useCases)) useCases = [useCases];
                for (var u = 0; u < useCases.length; u++) {
                    var uc = useCases[u];
                    var name = uc.useCaseName || '';
                    var avail = uc.useCaseAvailable === true;
                    var cls = avail ? 'uc-pill uc-available' : 'uc-pill uc-unavailable';
                    html += '<span class="' + cls + '">' + escapeHtml(name) + '</span> ';
                }
                html += '</div>';
            }
        }
        return html || '<div class="empty-state">No use case data</div>';
    }

    function renderSubscriptionOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            var sd = cmd.nodeManagementSubscriptionData || cmd.nodeManagementSubscriptionRequestCall || cmd.nodeManagementSubscriptionDeleteCall;
            if (!sd) continue;
            var entries = sd.subscriptionEntry || [];
            if (!Array.isArray(entries)) entries = [entries];
            for (var j = 0; j < entries.length; j++) {
                var e = entries[j];
                var server = e.serverAddress || {};
                var client = e.clientAddress || {};
                rows.push({
                    'Feature Type': (server.featureType || e.serverFeatureType || ''),
                    Client: client.feature !== undefined ? String(client.entity || '') + '.' + String(client.feature) : JSON.stringify(client),
                    Server: server.feature !== undefined ? String(server.entity || '') + '.' + String(server.feature) : JSON.stringify(server)
                });
            }
        }
        if (rows.length === 0) return '<div class="empty-state">No subscription entries</div>';
        return overviewTable('Subscriptions', ['Feature Type', 'Client', 'Server'], rows);
    }

    function renderBindingOverview(cmds) {
        var rows = [];
        for (var i = 0; i < cmds.length; i++) {
            var cmd = cmds[i];
            var bd = cmd.nodeManagementBindingData || cmd.nodeManagementBindingRequestCall || cmd.nodeManagementBindingDeleteCall;
            if (!bd) continue;
            var entries = bd.bindingEntry || [];
            if (!Array.isArray(entries)) entries = [entries];
            for (var j = 0; j < entries.length; j++) {
                var e = entries[j];
                var server = e.serverAddress || {};
                var client = e.clientAddress || {};
                rows.push({
                    'Feature Type': (server.featureType || e.serverFeatureType || ''),
                    Client: client.feature !== undefined ? String(client.entity || '') + '.' + String(client.feature) : JSON.stringify(client),
                    Server: server.feature !== undefined ? String(server.entity || '') + '.' + String(server.feature) : JSON.stringify(server)
                });
            }
        }
        if (rows.length === 0) return '<div class="empty-state">No binding entries</div>';
        return overviewTable('Bindings', ['Feature Type', 'Client', 'Server'], rows);
    }

    // --- Register Overview Renderers ---
    registerOverview('LoadControlLimitListData',   { needsDescs: true,  render: renderLoadControlOverview });
    registerOverview('MeasurementListData',         { needsDescs: true,  render: renderMeasurementOverview });
    registerOverview('SetpointListData',            { render: renderSetpointOverview });
    registerOverview('DeviceDiagnosisHeartbeatData', { render: renderHeartbeatOverview });
    registerOverview('DeviceDiagnosisStateData',    { render: renderDiagnosisStateOverview });
    registerOverview('DeviceClassificationManufacturerData', { render: renderManufacturerOverview });
    registerOverview('MeasurementDescriptionListData', { render: renderMeasurementDescOverview });
    registerOverview('LoadControlLimitDescriptionListData', { render: renderLimitDescOverview });
    registerOverview('ElectricalConnectionParameterDescriptionListData', { render: renderElecParamDescOverview });
    registerOverview('SetpointDescriptionListData', { render: renderSetpointDescOverview });
    registerOverview('NodeManagementDetailedDiscoveryData', { render: renderDiscoveryOverview });
    registerOverview('NodeManagementUseCaseData',   { render: renderUseCaseOverview });

    registerOverviewPattern(
        function(fs) { return fs.indexOf('Subscription') !== -1 || fs.indexOf('subscription') !== -1; },
        { render: renderSubscriptionOverview }
    );
    registerOverviewPattern(
        function(fs) { return fs.indexOf('Binding') !== -1 || fs.indexOf('binding') !== -1; },
        { render: renderBindingOverview }
    );

    function setupTabs(msg) {
        document.querySelectorAll('.detail-panel .tab').forEach(tab => {
            tab.onclick = function() {
                document.querySelectorAll('.detail-panel .tab').forEach(t => t.classList.remove('active'));
                this.classList.add('active');
                const content = document.getElementById('detail-content');
                const tabName = this.dataset.tab;

                if (tabName === 'overview') {
                    renderOverviewTab(msg, content);
                    return;
                } else if (tabName === 'decoded') {
                    let json = msg.spinePayload || msg.normalizedJson || msg.shipPayload || '{}';
                    if (typeof json === 'string') {
                        try { json = JSON.parse(json); } catch(e) {}
                    }
                    content.innerHTML = '<pre class="json-display">' + syntaxHighlight(JSON.stringify(json, null, 2)) + '</pre>';
                    return;
                } else if (tabName === 'headers') {
                    const headers = {
                        shipMsgType: msg.shipMsgType,
                        cmdClassifier: msg.cmdClassifier,
                        functionSet: msg.functionSet,
                        msgCounter: msg.msgCounter,
                        msgCounterRef: msg.msgCounterRef,
                        deviceSource: msg.deviceSource,
                        deviceDest: msg.deviceDest,
                        entitySource: msg.entitySource,
                        entityDest: msg.entityDest,
                        featureSource: msg.featureSource,
                        featureDest: msg.featureDest,
                        parseError: msg.parseError
                    };
                    content.innerHTML = '<pre class="json-display">' + syntaxHighlight(JSON.stringify(headers, null, 2)) + '</pre>';
                } else if (tabName === 'related') {
                    loadRelatedMessages(msg.traceId, msg.id, content);
                }
            };
        });
    }

    // --- Related Messages ---
    async function loadRelatedMessages(traceId, msgId, container) {
        container.innerHTML = '<div class="empty-state">Loading...</div>';
        try {
            var results = await Promise.all([
                fetch('/api/traces/' + traceId + '/messages/' + msgId + '/related').then(function(r) { return r.json(); }),
                fetch('/api/traces/' + traceId + '/messages/' + msgId + '/conversation?limit=30').then(function(r) { return r.json(); })
            ]);
            var related = results[0];
            var conv = results[1];

            if (related.length === 0 && conv.total === 0) {
                container.innerHTML = '<div class="empty-state">No related messages found</div>';
                return;
            }

            var html = '';

            // Section 1: Direct Correlation
            if (related.length > 0) {
                html += '<div class="related-section">';
                html += '<div class="related-section-header">Direct Correlation <span class="count-badge">' + related.length + '</span></div>';
                html += '<ul class="related-list">';
                for (var i = 0; i < related.length; i++) {
                    var r = related[i];
                    var m = r.message;
                    html += '<li class="related-item" onclick="showDetail(' + m.traceId + ',' + m.id + ')">';
                    html += '<span>#' + m.sequenceNum + ' ' + (m.cmdClassifier || '') + ' ' + (m.functionSet || '') + '</span>';
                    html += '<span class="related-meta">';
                    html += '<span class="related-type">' + r.relationship + '</span>';
                    if (r.latencyMs != null) {
                        var latStr = r.latencyMs < 1 ? r.latencyMs.toFixed(2) : r.latencyMs < 100 ? r.latencyMs.toFixed(1) : Math.round(r.latencyMs);
                        html += '<span class="related-latency">' + latStr + ' ms</span>';
                    }
                    if (r.resultStatus) {
                        html += '<span class="related-result ' + r.resultStatus + '">' + r.resultStatus + '</span>';
                    }
                    html += '</span>';
                    html += '</li>';
                }
                html += '</ul></div>';
            }

            // Section 2: Conversation
            if (conv.total > 0) {
                html += '<div class="related-section">';
                html += '<div class="related-section-header">Conversation <span class="count-badge">' + conv.total + '</span></div>';
                html += '<ul class="related-list">';
                for (var j = 0; j < conv.messages.length; j++) {
                    var cm = conv.messages[j];
                    var isCurrent = cm.id === msgId;
                    var ts = new Date(cm.timestamp);
                    var timeStr = ts.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit', second:'2-digit', fractionalSecondDigits: 3});
                    html += '<li class="conversation-item' + (isCurrent ? ' conversation-current' : '') + '" onclick="showDetail(' + cm.traceId + ',' + cm.id + ')">';
                    html += '<span class="conv-time">' + timeStr + '</span>';
                    html += '<span class="conv-direction">' + (cm.deviceSource || '?') + ' &rarr; ' + (cm.deviceDest || '?') + '</span>';
                    html += '<span class="overview-badge-cmd overview-badge-' + (cm.cmdClassifier || 'unknown') + '">' + (cm.cmdClassifier || '') + '</span>';
                    html += '<span>#' + cm.sequenceNum + '</span>';
                    html += '</li>';
                }
                html += '</ul>';
                if (conv.messages.length < conv.total) {
                    html += '<div class="conversation-more">Showing ' + conv.messages.length + ' of ' + conv.total + '</div>';
                }
                html += '</div>';
            }

            container.innerHTML = html;
        } catch (e) {
            container.innerHTML = '<div class="empty-state">Failed to load related messages</div>';
        }
    }

    // --- Trace Actions Menu ---
    (function() {
        var btn = document.getElementById('trace-menu-btn');
        var dropdown = document.getElementById('trace-menu-dropdown');
        if (!btn || !dropdown) return;

        btn.addEventListener('click', function(e) {
            e.stopPropagation();
            var open = dropdown.classList.toggle('open');
            btn.classList.toggle('open', open);
        });

        document.addEventListener('click', function() {
            dropdown.classList.remove('open');
            btn.classList.remove('open');
        });
    })();

    // --- Resizable Detail Panel ---
    (function() {
        var handle = document.getElementById('resize-handle');
        var panel = document.getElementById('detail-panel');
        if (!handle || !panel) return;

        var startY, startH;

        handle.addEventListener('mousedown', function(e) {
            e.preventDefault();
            startY = e.clientY;
            startH = panel.offsetHeight;
            handle.classList.add('dragging');
            document.addEventListener('mousemove', onDrag);
            document.addEventListener('mouseup', onStop);
        });

        function onDrag(e) {
            var newH = startH - (e.clientY - startY);
            if (newH < 80) newH = 80;
            var maxH = panel.parentElement.offsetHeight - 200;
            if (newH > maxH) newH = maxH;
            panel.style.height = newH + 'px';
        }

        function onStop() {
            handle.classList.remove('dragging');
            document.removeEventListener('mousemove', onDrag);
            document.removeEventListener('mouseup', onStop);
            try { localStorage.setItem('detailPanelHeight', panel.style.height); } catch(e) {}
        }

        // Restore saved height
        try {
            var saved = localStorage.getItem('detailPanelHeight');
            if (saved) panel.style.height = saved;
        } catch(e) {}
    })();

    // --- Resizable Table Columns ---
    (function() {
        var table = document.getElementById('message-table');
        if (!table) return;

        var headers = table.querySelectorAll('thead th');
        // Skip bookmark column (0)
        var skip = { 0: true };

        headers.forEach(function(th, i) {
            if (skip[i]) return;

            var handle = document.createElement('div');
            handle.className = 'col-resize-handle';
            th.appendChild(handle);

            var startX, startW;

            handle.addEventListener('mousedown', function(e) {
                e.preventDefault();
                e.stopPropagation();
                startX = e.clientX;
                startW = th.offsetWidth;
                handle.classList.add('dragging');
                document.addEventListener('mousemove', onDrag);
                document.addEventListener('mouseup', onStop);
            });

            function onDrag(e) {
                var w = startW + (e.clientX - startX);
                if (w < 40) w = 40;
                th.style.width = w + 'px';
            }

            function onStop() {
                handle.classList.remove('dragging');
                document.removeEventListener('mousemove', onDrag);
                document.removeEventListener('mouseup', onStop);
                saveColumnWidths();
            }
        });

        function saveColumnWidths() {
            var widths = [];
            headers.forEach(function(th) {
                widths.push(th.style.width || '');
            });
            try { localStorage.setItem('colWidths', JSON.stringify(widths)); } catch(e) {}
        }

        // Restore saved widths
        try {
            var saved = localStorage.getItem('colWidths');
            if (saved) {
                var widths = JSON.parse(saved);
                headers.forEach(function(th, i) {
                    if (widths[i]) th.style.width = widths[i];
                });
            }
        } catch(e) {}
    })();

    // --- Filter Toolbar ---
    const btnClearFilter = document.getElementById('btn-clear-filter');

    // Debounce helper for text inputs
    var filterTimer = null;
    function debouncedApplyFilter() {
        clearTimeout(filterTimer);
        filterTimer = setTimeout(applyFilter, 250);
    }

    // Auto-apply: text inputs debounce, selects fire immediately
    ['filter-search'].forEach(function(id) {
        var el = document.getElementById(id);
        if (el) el.addEventListener('input', debouncedApplyFilter);
    });
    ['filter-cmd', 'filter-usecase'].forEach(function(id) {
        var el = document.getElementById(id);
        if (el) el.addEventListener('change', function() { applyFilter(); });
    });

    if (btnClearFilter) {
        btnClearFilter.addEventListener('click', clearFilter);
    }

    function populateUseCaseDropdown(data) {
        var select = document.getElementById('filter-usecase');
        if (!select) return;
        // Clear existing options except the first
        while (select.options.length > 1) select.remove(1);
        useCaseContextMap = {};
        for (var i = 0; i < data.length; i++) {
            var uc = data[i];
            useCaseContextMap[uc.abbreviation] = uc;
            var opt = document.createElement('option');
            opt.value = uc.abbreviation;
            opt.textContent = uc.abbreviation + ' \u2014 ' + uc.devices.length + ' device' + (uc.devices.length !== 1 ? 's' : '');
            select.appendChild(opt);
        }
    }

    function buildFilterParams() {
        const params = new URLSearchParams();
        const search = document.getElementById('filter-search');
        const cmd = document.getElementById('filter-cmd');
        const ucSelect = document.getElementById('filter-usecase');

        if (search && search.value.trim()) params.set('search', search.value.trim());
        if (cmd && cmd.value) params.set('cmdClassifier', cmd.value);

        if (ucSelect && ucSelect.value) {
            var uc = useCaseContextMap[ucSelect.value];
            if (uc) {
                params.set('functionSet', uc.functionSets.join(','));
                if (uc.devices.length === 1) {
                    params.set('device', uc.devices[0]);
                }
            }
        }
        return params;
    }

    function buildRowHTML(msg) {
        var ts = new Date(msg.timestamp);
        var timeStr = ts.toTimeString().substring(0, 8) + '.' + String(ts.getMilliseconds()).padStart(3, '0');
        var arrow = msg.direction === 'incoming' ? '\u2190' : '\u2192';
        var cls = msg.cmdClassifier || '';
        var rowClass = 'msg-row' + (orphanedIds.has(msg.id) ? ' msg-orphan' : '');
        return '<tr class="' + rowClass + '" data-id="' + msg.id + '" onclick="showDetail(' + msg.traceId + ',' + msg.id + ')">' +
            '<td class="col-bookmark" id="bm-' + msg.id + '"></td>' +
            '<td class="col-seq">' + (msg.sequenceNum || '') + '</td>' +
            '<td>' + timeStr + '</td>' +
            '<td class="col-comm">' + (msg.deviceSource || '') + ' <span class="dir-arrow dir-' + msg.direction + '">' + arrow + '</span> ' + (msg.deviceDest || '') + '</td>' +
            '<td class="cmd-' + cls + '">' + cls + '</td>' +
            '<td>' + (msg.functionSet || msg.shipMsgType || '') + '</td>' +
            '</tr>';
    }

    async function applyFilter() {
        if (typeof window.TRACE_ID === 'undefined') return;

        currentOffset = 0;
        const params = buildFilterParams();
        params.set('limit', String(PAGE_SIZE));
        params.set('offset', '0');
        try {
            const resp = await fetch('/api/traces/' + window.TRACE_ID + '/messages?' + params.toString());
            const messages = await resp.json();
            var totalCount = parseInt(resp.headers.get('X-Total-Count')) || 0;
            var unfilteredCount = parseInt(resp.headers.get('X-Unfiltered-Count')) || totalCount;
            lastTotalCount = totalCount;
            const tbody = document.getElementById('message-tbody');
            if (tbody) {
                var html = '';
                for (var i = 0; i < messages.length; i++) {
                    html += buildRowHTML(messages[i]);
                }
                tbody.innerHTML = html;
            }
            currentOffset = messages.length;
            updateFilterIndicator(totalCount, unfilteredCount);
            updateLoadMoreButton(messages.length, totalCount);
            updateStatusBar();
        } catch (e) {
            console.error('Filter failed:', e);
        }
    }

    function clearFilter() {
        const inputs = ['filter-search'];
        inputs.forEach(id => {
            const el = document.getElementById(id);
            if (el) el.value = '';
        });
        const selects = ['filter-cmd', 'filter-usecase'];
        selects.forEach(id => {
            const el = document.getElementById(id);
            if (el) el.value = '';
        });
        currentOffset = 0;
        var toolbar = document.getElementById('toolbar');
        if (toolbar) toolbar.classList.remove('filter-active');
        applyFilter();
    }

    // --- Filter Active Indicator ---
    // filteredTotal: count of messages matching the current filter
    // unfilteredTotal: count of all messages in the trace (no filter)
    function updateFilterIndicator(filteredTotal, unfilteredTotal) {
        var toolbar = document.getElementById('toolbar');
        var statusMsgs = document.getElementById('status-messages');

        if (filteredTotal < unfilteredTotal) {
            if (toolbar) toolbar.classList.add('filter-active');
            if (statusMsgs) {
                statusMsgs.textContent = 'Showing ' + filteredTotal + ' of ' + unfilteredTotal + ' messages';
                statusMsgs.classList.add('status-filtered');
            }
        } else {
            if (toolbar) toolbar.classList.remove('filter-active');
            if (statusMsgs) {
                statusMsgs.classList.remove('status-filtered');
            }
        }
    }

    // --- Load More Button ---
    function updateLoadMoreButton(displayed, total) {
        var btn = document.getElementById('load-more-btn');
        if (!btn) return;
        var remaining = total - displayed;
        if (remaining > 0) {
            btn.style.display = '';
            btn.textContent = 'Load more (' + remaining + ' remaining)';
        } else {
            btn.style.display = 'none';
        }
    }

    (function() {
        var btn = document.getElementById('load-more-btn');
        if (!btn) return;
        btn.addEventListener('click', async function() {
            if (typeof window.TRACE_ID === 'undefined') return;
            var params = buildFilterParams();
            params.set('limit', String(PAGE_SIZE));
            params.set('offset', String(currentOffset));
            try {
                var resp = await fetch('/api/traces/' + window.TRACE_ID + '/messages?' + params.toString());
                var messages = await resp.json();
                var totalCount = parseInt(resp.headers.get('X-Total-Count')) || lastTotalCount;
                lastTotalCount = totalCount;
                var tbody = document.getElementById('message-tbody');
                if (tbody && messages.length > 0) {
                    var html = '';
                    for (var i = 0; i < messages.length; i++) {
                        html += buildRowHTML(messages[i]);
                    }
                    tbody.insertAdjacentHTML('beforeend', html);
                }
                currentOffset += messages.length;
                updateLoadMoreButton(currentOffset, totalCount);
                updateStatusBar();
            } catch (e) {
                console.error('Load more failed:', e);
            }
        });
    })();

    // Initial load-more button state for server-rendered pages
    (function() {
        if (typeof window.TRACE_ID === 'undefined') return;
        var tbody = document.getElementById('message-tbody');
        var total = typeof window.TOTAL_MESSAGES !== 'undefined' ? window.TOTAL_MESSAGES : 0;
        if (tbody && total > 0) {
            var displayed = tbody.children.length;
            currentOffset = displayed;
            updateLoadMoreButton(displayed, total);
        }
    })();

    // --- Filter Presets (gear-icon dropdown) ---
    const btnPresets = document.getElementById('btn-presets');
    const presetsDropdown = document.getElementById('presets-dropdown');
    const presetList = document.getElementById('preset-list');
    const btnSavePreset = document.getElementById('btn-save-preset');

    if (btnPresets && presetsDropdown) {
        btnPresets.addEventListener('click', function(e) {
            e.stopPropagation();
            presetsDropdown.classList.toggle('open');
        });

        document.addEventListener('click', function() {
            presetsDropdown.classList.remove('open');
        });

        presetsDropdown.addEventListener('click', function(e) {
            e.stopPropagation();
        });
    }

    if (btnSavePreset) {
        btnSavePreset.addEventListener('click', async () => {
            const name = prompt('Preset name:');
            if (!name) return;

            const params = buildFilterParams();
            params.delete('limit');
            const filterObj = {};
            for (const [k, v] of params) { filterObj[k] = v; }

            try {
                await fetch('/api/presets', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({name: name, filter: JSON.stringify(filterObj)})
                });
                loadPresets();
            } catch (e) {
                alert('Save preset failed: ' + e.message);
            }
            if (presetsDropdown) presetsDropdown.classList.remove('open');
        });
    }

    async function loadPresets() {
        if (!presetList) return;
        try {
            const resp = await fetch('/api/presets');
            const presets = await resp.json();
            presetList.innerHTML = '';
            presets.forEach(p => {
                const row = document.createElement('div');
                row.className = 'toolbar-presets-row';

                const btn = document.createElement('button');
                btn.className = 'toolbar-presets-item';
                btn.textContent = p.name;
                btn.dataset.filter = p.filter;
                btn.addEventListener('click', function() {
                    try {
                        const filter = JSON.parse(p.filter);
                        var searchEl = document.getElementById('filter-search');
                        var cmdEl = document.getElementById('filter-cmd');
                        if (searchEl) searchEl.value = filter.search || '';
                        if (cmdEl) cmdEl.value = filter.cmdClassifier || '';
                        applyFilter();
                    } catch (e) {
                        console.error('Apply preset failed:', e);
                    }
                    if (presetsDropdown) presetsDropdown.classList.remove('open');
                });

                const del = document.createElement('button');
                del.className = 'toolbar-presets-delete';
                del.innerHTML = '&times;';
                del.title = 'Delete preset';
                del.addEventListener('click', async function(e) {
                    e.stopPropagation();
                    try {
                        await fetch('/api/presets/' + p.id, { method: 'DELETE' });
                        loadPresets();
                    } catch (err) {
                        console.error('Delete preset failed:', err);
                    }
                });

                row.appendChild(btn);
                row.appendChild(del);
                presetList.appendChild(row);
            });
        } catch (e) {
            console.error('Load presets failed:', e);
        }
    }

    // --- Sidebar Tabs ---
    document.querySelectorAll('.sidebar-tab').forEach(tab => {
        tab.addEventListener('click', function() {
            document.querySelectorAll('.sidebar-tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.sidebar-panel').forEach(p => p.classList.remove('active'));
            this.classList.add('active');
            const panel = document.getElementById(this.dataset.panel);
            if (panel) panel.classList.add('active');
        });
    });

    // --- Sidebar Collapse ---
    var sidebarToggle = document.getElementById('sidebar-toggle');
    var sidebar = document.querySelector('.sidebar');

    if (sidebarToggle && sidebar) {
        if (localStorage.getItem('sidebarCollapsed') === 'true') {
            sidebar.classList.add('collapsed');
            sidebarToggle.innerHTML = '&raquo;';
        }

        sidebarToggle.addEventListener('click', function() {
            sidebar.classList.toggle('collapsed');
            var isCollapsed = sidebar.classList.contains('collapsed');
            sidebarToggle.innerHTML = isCollapsed ? '&raquo;' : '&laquo;';
            localStorage.setItem('sidebarCollapsed', isCollapsed);
        });
    }

    // --- Keyboard Shortcuts ---
    function navigateMessages(direction) {
        var rows = Array.from(document.querySelectorAll('.msg-row'));
        if (rows.length === 0) return;
        var current = document.querySelector('.msg-row.selected');
        var currentIndex = current ? rows.indexOf(current) : -1;
        var newIndex;
        if (currentIndex === -1) {
            newIndex = direction === 1 ? 0 : rows.length - 1;
        } else {
            newIndex = currentIndex + direction;
        }
        if (newIndex < 0) newIndex = 0;
        if (newIndex >= rows.length) newIndex = rows.length - 1;
        var newRow = rows[newIndex];
        var msgId = parseInt(newRow.dataset.id);
        showDetail(window.TRACE_ID, msgId);
        newRow.scrollIntoView({ block: 'nearest' });
    }

    function openJumpDialog() {
        var overlay = document.getElementById('jump-dialog');
        var input = document.getElementById('jump-input');
        if (!overlay || !input) return;
        overlay.style.display = 'flex';
        input.value = '';
        input.focus();

        function onKey(e) {
            if (e.key === 'Enter') {
                e.preventDefault();
                input.removeEventListener('keydown', onKey);
                var val = input.value.trim();
                overlay.style.display = 'none';
                if (!val) return;
                var rows = Array.from(document.querySelectorAll('.msg-row'));
                for (var i = 0; i < rows.length; i++) {
                    var cells = rows[i].querySelectorAll('td');
                    // Column 1 (after bookmark) = sequence number, last column = msgCounter
                    var seqText = cells[1] ? cells[1].textContent.trim() : '';
                    var ctrText = cells[cells.length - 1] ? cells[cells.length - 1].textContent.trim() : '';
                    if (seqText === val || ctrText === val) {
                        var msgId = parseInt(rows[i].dataset.id);
                        showDetail(window.TRACE_ID, msgId);
                        rows[i].scrollIntoView({ block: 'nearest' });
                        return;
                    }
                }
            } else if (e.key === 'Escape') {
                e.preventDefault();
                input.removeEventListener('keydown', onKey);
                overlay.style.display = 'none';
            }
        }
        input.addEventListener('keydown', onKey);
    }

    function showKbdHelp() {
        var overlay = document.getElementById('kbd-help');
        var content = document.getElementById('kbd-help-content');
        if (!overlay || !content) return;
        var mod = /Mac|iPhone|iPad/.test(navigator.platform) ? '\u2318' : 'Ctrl';
        content.innerHTML =
            '<table class="kbd-table">' +
            '<tr><td><kbd>j</kbd> / <kbd>k</kbd> or <kbd>\u2193</kbd> / <kbd>\u2191</kbd></td><td>Next / previous message</td></tr>' +
            '<tr><td><kbd>' + mod + '</kbd>+<kbd>F</kbd></td><td>Focus find input</td></tr>' +
            '<tr><td><kbd>' + mod + '</kbd>+<kbd>L</kbd></td><td>Focus filter dropdown</td></tr>' +
            '<tr><td><kbd>' + mod + '</kbd>+<kbd>G</kbd></td><td>Jump to message by number</td></tr>' +
            '<tr><td><kbd>?</kbd></td><td>Show this help</td></tr>' +
            '<tr><td><kbd>Esc</kbd></td><td>Close dialog / blur input</td></tr>' +
            '</table>';
        overlay.style.display = 'flex';
    }

    function closeAllDialogs() {
        var ids = ['jump-dialog', 'kbd-help'];
        for (var i = 0; i < ids.length; i++) {
            var el = document.getElementById(ids[i]);
            if (el) el.style.display = 'none';
        }
    }

    // --- Find (jump between matches in visible rows) ---
    var findMatches = [];
    var findIndex = -1;

    function clearFind() {
        var input = document.getElementById('find-input');
        if (input) input.value = '';
        clearFindHighlights();
        findMatches = [];
        findIndex = -1;
        updateFindCount();
        if (input) input.blur();
    }

    function clearFindHighlights() {
        var rows = document.querySelectorAll('.msg-row.find-match, .msg-row.find-current');
        for (var i = 0; i < rows.length; i++) {
            rows[i].classList.remove('find-match', 'find-current');
        }
    }

    function updateFindCount() {
        var el = document.getElementById('find-count');
        if (!el) return;
        if (findMatches.length === 0) {
            var input = document.getElementById('find-input');
            el.textContent = (input && input.value.trim()) ? 'No matches' : '';
        } else {
            el.textContent = (findIndex + 1) + ' of ' + findMatches.length;
        }
    }

    function runFind() {
        clearFindHighlights();
        findMatches = [];
        findIndex = -1;

        var input = document.getElementById('find-input');
        if (!input) return;
        var term = input.value.trim().toLowerCase();
        if (!term) { updateFindCount(); return; }

        var rows = document.querySelectorAll('#message-tbody .msg-row');
        for (var i = 0; i < rows.length; i++) {
            var text = rows[i].textContent.toLowerCase();
            if (text.indexOf(term) !== -1) {
                rows[i].classList.add('find-match');
                findMatches.push(rows[i]);
            }
        }

        updateFindCount();
        if (findMatches.length > 0) {
            findIndex = 0;
            highlightFindCurrent();
        }
    }

    function highlightFindCurrent() {
        // Remove previous current
        var prev = document.querySelector('.msg-row.find-current');
        if (prev) prev.classList.remove('find-current');

        if (findIndex < 0 || findIndex >= findMatches.length) return;
        var row = findMatches[findIndex];
        row.classList.add('find-current');
        row.scrollIntoView({ block: 'center', behavior: 'smooth' });
        updateFindCount();
    }

    function findNext() {
        if (findMatches.length === 0) return;
        findIndex = (findIndex + 1) % findMatches.length;
        highlightFindCurrent();
    }

    function findPrev() {
        if (findMatches.length === 0) return;
        findIndex = (findIndex - 1 + findMatches.length) % findMatches.length;
        highlightFindCurrent();
    }

    (function() {
        var input = document.getElementById('find-input');
        var btnNext = document.getElementById('find-next');
        var btnPrev = document.getElementById('find-prev');

        if (input) {
            var findTimer = null;
            input.addEventListener('input', function() {
                clearTimeout(findTimer);
                findTimer = setTimeout(runFind, 150);
            });
            input.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' && e.shiftKey) {
                    e.preventDefault();
                    findPrev();
                } else if (e.key === 'Enter') {
                    e.preventDefault();
                    findNext();
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    clearFind();
                }
            });
        }
        if (btnNext) btnNext.addEventListener('click', findNext);
        if (btnPrev) btnPrev.addEventListener('click', findPrev);
    })();

    document.addEventListener('keydown', function(e) {
        var tag = e.target.tagName;
        var isInput = (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT');

        // Modifier shortcuts — fire regardless of focus
        var mod = e.metaKey || e.ctrlKey;
        if (mod && e.key === 'f') {
            e.preventDefault();
            var fi = document.getElementById('find-input');
            if (fi) { fi.focus(); fi.select(); }
            return;
        }
        if (mod && e.key === 'l') {
            e.preventDefault();
            var filterCmd = document.getElementById('filter-cmd');
            if (filterCmd) filterCmd.focus();
            return;
        }
        if (mod && e.key === 'g') {
            e.preventDefault();
            openJumpDialog();
            return;
        }

        // Escape — close dialogs or blur input
        if (e.key === 'Escape') {
            var jumpDialog = document.getElementById('jump-dialog');
            var kbdHelp = document.getElementById('kbd-help');
            if ((jumpDialog && jumpDialog.style.display !== 'none') ||
                (kbdHelp && kbdHelp.style.display !== 'none')) {
                closeAllDialogs();
            } else if (isInput) {
                e.target.blur();
            }
            return;
        }

        // Plain-key shortcuts — only when not in an input
        if (isInput) return;

        if (e.key === 'j' || e.key === 'ArrowDown') {
            e.preventDefault();
            navigateMessages(1);
            return;
        }
        if (e.key === 'k' || e.key === 'ArrowUp') {
            e.preventDefault();
            navigateMessages(-1);
            return;
        }
        if (e.key === '?') {
            showKbdHelp();
            return;
        }
    });

    // Close overlays on backdrop click
    document.addEventListener('click', function(e) {
        if (e.target.classList.contains('kbd-overlay')) {
            e.target.style.display = 'none';
        }
    });

    // --- Device Panel ---
    async function loadDevicePanel() {
        var container = document.getElementById('device-panel');
        if (!container) return;

        var mdnsDevices = [];
        try {
            var resp = await fetch('/api/mdns/devices');
            mdnsDevices = await resp.json();
            if (!Array.isArray(mdnsDevices)) mdnsDevices = [];
        } catch (e) {
            mdnsDevices = [];
        }

        var recentTargets = loadRecentTargets();
        var html = '';

        if (mdnsDevices.length > 0) {
            html += '<div class="device-list-section">Discovered Devices</div>';
            for (var i = 0; i < mdnsDevices.length; i++) {
                var dev = mdnsDevices[i];
                var addr = '';
                if (dev.addresses && dev.addresses.length > 0) {
                    addr = dev.addresses[0];
                }
                var port = dev.port || 4712;
                var name = dev.brand || dev.model || dev.deviceType || dev.ski || 'Unknown Device';
                var meta = addr ? addr + ':' + port : '';
                var onlineCls = dev.online ? 'device-online' : 'device-offline';
                var onlineLabel = dev.online ? 'online' : 'offline';
                html += '<div class="device-entry" onclick="useDevice(\'tcp\',\'' + escapeHtml(addr) + '\',' + port + ')">';
                html += '<div class="device-entry-name">' + escapeHtml(name) + '</div>';
                html += '<div class="device-entry-meta">';
                html += '<span>' + escapeHtml(meta) + '</span>';
                html += '<span class="device-status-badge ' + onlineCls + '">' + onlineLabel + '</span>';
                html += '</div>';
                html += '</div>';
            }
        }

        if (recentTargets.length > 0) {
            html += '<div class="device-list-section">Recent Targets</div>';
            for (var j = 0; j < recentTargets.length; j++) {
                var t = recentTargets[j];
                var label = '';
                var modeBadge = t.mode || 'udp';
                if (t.mode === 'logtail') {
                    label = t.path || '';
                } else {
                    label = (t.host || '') + ':' + (t.port || '');
                }
                html += '<div class="device-entry" onclick="useDevice(\'' + escapeHtml(t.mode || 'udp') + '\',\'' + escapeHtml(t.host || t.path || '') + '\',' + (t.port || 0) + ')">';
                html += '<div class="device-entry-name">' + escapeHtml(label) + '</div>';
                html += '<div class="device-entry-meta">';
                html += '<span class="device-mode-badge">' + escapeHtml(modeBadge) + '</span>';
                html += '</div>';
                html += '</div>';
            }
        }

        if (mdnsDevices.length === 0 && recentTargets.length === 0) {
            html = '<div class="empty-state">No devices found</div>';
        }

        container.innerHTML = html;
    }

    window.useDevice = function(mode, host, port) {
        if (captureMode) {
            captureMode.value = mode;
            captureMode.dispatchEvent(new Event('change'));
        }
        if (mode === 'logtail') {
            if (logFileInput) logFileInput.value = host;
        } else if (mode === 'tcp') {
            if (tcpHostInput) tcpHostInput.value = host;
            if (tcpPortInput) tcpPortInput.value = port || 54546;
        } else {
            if (hostInput) hostInput.value = host;
            if (portInput) portInput.value = port || 4712;
        }
    };

    // --- Bookmarks ---
    async function loadBookmarks() {
        if (typeof window.TRACE_ID === 'undefined') return;
        const container = document.getElementById('bookmark-list');
        if (!container) return;

        try {
            const resp = await fetch('/api/traces/' + window.TRACE_ID + '/bookmarks');
            const bookmarks = await resp.json();
            if (bookmarks.length === 0) {
                container.innerHTML = '<div class="empty-state">No bookmarks yet</div>';
                return;
            }
            let html = '';
            bookmarks.forEach(b => {
                const color = b.color || getComputedStyle(document.documentElement).getPropertyValue('--bookmark-default').trim();
                html += '<div class="bookmark-entry" onclick="showDetail(' + window.TRACE_ID + ',' + b.messageId + ')">';
                html += '<span class="bookmark-label">' + escapeHtml(b.label || 'Message #' + b.messageId) + '</span>';
                html += '<span class="bookmark-dot" style="background:' + escapeHtml(color) + '"></span>';
                html += '</div>';

                // Mark the message row
                const bmCell = document.getElementById('bm-' + b.messageId);
                if (bmCell) {
                    bmCell.innerHTML = '<span class="bookmark-badge" style="background:' + escapeHtml(color) + '"></span>';
                }
            });
            container.innerHTML = html;
        } catch (e) {
            container.innerHTML = '<div class="empty-state">Failed to load bookmarks</div>';
        }
    }

    const btnBookmark = document.getElementById('btn-bookmark-msg');
    if (btnBookmark) {
        btnBookmark.addEventListener('click', async () => {
            if (!selectedMsgId || typeof window.TRACE_ID === 'undefined') return;

            const label = prompt('Bookmark label:', '');
            if (label === null) return;

            try {
                await fetch('/api/traces/' + window.TRACE_ID + '/bookmarks', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({messageId: selectedMsgId, label: label, color: getComputedStyle(document.documentElement).getPropertyValue('--bookmark-default').trim()})
                });
                loadBookmarks();
            } catch (e) {
                alert('Bookmark failed: ' + e.message);
            }
        });
    }

    // --- Delete Trace ---
    window.deleteTrace = async function(traceId) {
        if (!confirm('Delete this trace and all its messages?')) return;
        try {
            await fetch('/api/traces/' + traceId, {method: 'DELETE'});
            window.location.href = '/';
        } catch (e) {
            alert('Delete failed: ' + e.message);
        }
    };

    // --- Sidebar Rename Trace ---
    window.renameTrace = async function(event, traceId, btn) {
        event.stopPropagation();
        event.preventDefault();

        var traceItem = btn.closest('.trace-item');
        var nameEl = traceItem ? traceItem.querySelector('.trace-name') : null;
        var currentName = nameEl ? nameEl.textContent.trim() : '';
        var newName = prompt('Rename trace:', currentName);
        if (!newName || newName === currentName) return;

        try {
            var resp = await fetch('/api/traces/' + traceId, {
                method: 'PATCH',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({name: newName})
            });
            if (resp.ok) {
                if (nameEl) nameEl.textContent = newName;
                // Also update the trace header if we're on that trace's page
                var header = document.querySelector('.trace-header h2');
                if (header && typeof window.TRACE_ID !== 'undefined' && window.TRACE_ID === traceId) {
                    header.textContent = newName;
                }
            } else {
                var err = await resp.json();
                alert('Rename failed: ' + (err.error || 'unknown error'));
            }
        } catch (e) {
            alert('Rename failed: ' + e.message);
        }
    };

    // --- Sidebar Delete Trace ---
    window.sidebarDeleteTrace = async function(event, traceId) {
        event.stopPropagation();
        event.preventDefault();

        if (!confirm('Delete this trace and all its messages?')) return;

        try {
            var resp = await fetch('/api/traces/' + traceId, {method: 'DELETE'});
            if (resp.ok) {
                // Remove the trace item from the sidebar
                var items = document.querySelectorAll('.trace-item');
                for (var i = 0; i < items.length; i++) {
                    if (items[i].getAttribute('href') === '/traces/' + traceId) {
                        items[i].remove();
                        break;
                    }
                }
                // If we deleted the currently active trace, redirect to index
                if (typeof window.TRACE_ID !== 'undefined' && window.TRACE_ID === traceId) {
                    window.location.href = '/';
                }
            } else {
                var err = await resp.json();
                alert('Delete failed: ' + (err.error || 'unknown error'));
            }
        } catch (e) {
            alert('Delete failed: ' + e.message);
        }
    };

    // --- Import ---
    const btnImport = document.getElementById('btn-import');
    const importFile = document.getElementById('import-file');

    if (btnImport && importFile) {
        btnImport.addEventListener('click', () => importFile.click());
        importFile.addEventListener('change', async () => {
            if (!importFile.files.length) return;
            await importEebtFile(importFile.files[0]);
        });
    }

    async function importEebtFile(file) {
        const formData = new FormData();
        formData.append('file', file);
        try {
            const resp = await fetch('/api/traces/import', {method: 'POST', body: formData});
            if (resp.ok) {
                const trace = await resp.json();
                window.location.href = '/traces/' + trace.id;
            } else {
                const err = await resp.json();
                alert('Import failed: ' + err.error);
            }
        } catch (e) {
            alert('Import failed: ' + e.message);
        }
    }

    // --- Drag and Drop (document-level, works on all pages) ---
    var dragOverlay = document.createElement('div');
    dragOverlay.className = 'drag-overlay';
    dragOverlay.innerHTML = '<div class="drag-overlay-content">Drop .eet or .log file to import</div>';
    document.body.appendChild(dragOverlay);

    var dragCounter = 0;

    document.addEventListener('dragenter', function(e) {
        e.preventDefault();
        if (!e.dataTransfer || !e.dataTransfer.types.includes('Files')) return;
        dragCounter++;
        dragOverlay.classList.add('visible');
    });
    document.addEventListener('dragover', function(e) {
        e.preventDefault();
        if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
    });
    document.addEventListener('dragleave', function(e) {
        e.preventDefault();
        dragCounter--;
        if (dragCounter <= 0) {
            dragCounter = 0;
            dragOverlay.classList.remove('visible');
        }
    });
    document.addEventListener('drop', function(e) {
        e.preventDefault();
        dragCounter = 0;
        dragOverlay.classList.remove('visible');
        if (e.dataTransfer && e.dataTransfer.files.length > 0) {
            importEebtFile(e.dataTransfer.files[0]);
        }
    });

    // --- Load from file button (inside drop zone) ---
    const btnLoadFile = document.getElementById('btn-load-file');
    const dropZoneFile = document.getElementById('drop-zone-file');
    if (btnLoadFile && dropZoneFile) {
        btnLoadFile.addEventListener('click', function() {
            dropZoneFile.click();
        });
        dropZoneFile.addEventListener('change', function() {
            if (this.files.length > 0) {
                importEebtFile(this.files[0]);
            }
        });
    }

    // --- Status Bar ---
    function updateStatusBar() {
        const tbody = document.getElementById('message-tbody');
        const statusMsgs = document.getElementById('status-messages');
        if (tbody && statusMsgs) {
            // Don't overwrite if filter indicator already set the text
            if (!statusMsgs.classList.contains('status-filtered')) {
                statusMsgs.textContent = 'Messages: ' + tbody.children.length;
            }
        }
    }

    // --- Utility Functions ---
    // Delegate to shared utilities from common.js
    var syntaxHighlight = window.EEBusTracer ? EEBusTracer.syntaxHighlight : function(j) { return j; };
    var escapeHtml = window.EEBusTracer ? EEBusTracer.escapeHtml : function(t) { return t; };


    // --- Auto-Scroll Toggle ---
    var scrollContainer = document.querySelector('.message-table-container');
    var autoScrollBtn = document.getElementById('auto-scroll-btn');

    if (scrollContainer) {
        scrollContainer.addEventListener('scroll', function() {
            var atBottom = scrollContainer.scrollHeight - scrollContainer.scrollTop - scrollContainer.clientHeight < 30;
            if (atBottom) {
                autoScroll = true;
                if (autoScrollBtn) {
                    autoScrollBtn.classList.add('active');
                    autoScrollBtn.classList.remove('has-new');
                }
            } else {
                autoScroll = false;
                if (autoScrollBtn) {
                    autoScrollBtn.classList.remove('active');
                }
            }
        });
    }

    if (autoScrollBtn) {
        autoScrollBtn.addEventListener('click', function() {
            if (scrollContainer) {
                scrollContainer.scrollTop = scrollContainer.scrollHeight;
            }
            autoScroll = true;
            autoScrollBtn.classList.add('active');
            autoScrollBtn.classList.remove('has-new');
        });
    }

    // Re-render active detail tab on theme change
    document.addEventListener('theme-changed', function() {
        if (currentMsg) {
            var activeTab = document.querySelector('.detail-panel .tab.active');
            if (activeTab) activeTab.click();
        }
    });

    updateStatusBar();
})();

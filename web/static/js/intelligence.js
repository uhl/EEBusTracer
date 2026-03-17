(function() {
    'use strict';

    const TRACE_ID = window.TRACE_ID;
    const apiFetch = EEBusTracer.apiFetch;
    const escapeHtml = EEBusTracer.escapeHtml;

    // Tab switching
    document.querySelectorAll('.intel-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.intel-tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.intel-panel').forEach(p => p.classList.remove('active'));
            tab.classList.add('active');
            document.getElementById('panel-' + tab.dataset.tab).classList.add('active');
        });
    });

    // Load all tabs
    loadUseCases();
    loadSubscriptions();
    loadMetrics();

    function loadUseCases() {
        apiFetch(`/api/traces/${TRACE_ID}/usecases`)
            .then(data => {
                const container = document.getElementById('panel-usecases');
                if (!data || data.length === 0) {
                    container.innerHTML = '<div class="empty-state">No use cases detected in this trace.</div>';
                    return;
                }
                let html = '<div class="intel-grid">';
                data.forEach(duc => {
                    html += '<div class="intel-card">';
                    html += `<div class="intel-card-header">`;
                    html += `<span class="intel-card-title">${escapeHtml(duc.deviceAddr)}</span>`;
                    html += `<span class="intel-actor-badge">${escapeHtml(duc.actor)}</span>`;
                    html += '</div>';
                    html += '<div class="intel-card-body">';
                    duc.useCases.forEach(uc => {
                        const cls = uc.available ? 'uc-available' : 'uc-unavailable';
                        html += `<span class="uc-pill ${cls}" title="${escapeHtml(uc.useCaseName)}">`;
                        html += escapeHtml(uc.abbreviation);
                        if (uc.useCaseVersion) html += ` v${escapeHtml(uc.useCaseVersion)}`;
                        html += '</span>';
                    });
                    html += '</div></div>';
                });
                html += '</div>';
                container.innerHTML = html;
            })
            .catch(err => {
                document.getElementById('panel-usecases').innerHTML =
                    `<div class="empty-state">Error loading use cases: ${escapeHtml(err.message)}</div>`;
            });
    }

    function loadSubscriptions() {
        Promise.all([
            apiFetch(`/api/traces/${TRACE_ID}/subscriptions`),
            apiFetch(`/api/traces/${TRACE_ID}/bindings`)
        ]).then(([subs, bindings]) => {
            const container = document.getElementById('panel-subscriptions');
            let html = '';

            html += '<h3 class="intel-section-title">Subscriptions (' + subs.length + ')</h3>';
            if (subs.length === 0) {
                html += '<div class="empty-state">No subscriptions found.</div>';
            } else {
                html += '<table class="intel-table"><thead><tr>';
                html += '<th>Status</th><th>Feature</th><th>Client</th><th>Server</th><th>Notifies</th><th>Last Notify</th>';
                html += '</tr></thead><tbody>';
                subs.forEach(s => {
                    let status = '<span class="sub-status sub-active">Active</span>';
                    if (!s.active) status = '<span class="sub-status sub-removed">Removed</span>';
                    if (s.stale) status = '<span class="sub-status sub-stale">Stale</span>';
                    html += '<tr>';
                    html += `<td>${status}</td>`;
                    html += `<td>${s.serverFeatureType ? escapeHtml(s.serverFeatureType) : '-'}</td>`;
                    html += `<td>${formatDeviceFeature(s.clientDevice, s.clientFeature)}</td>`;
                    html += `<td>${formatDeviceFeature(s.serverDevice, s.serverFeature)}</td>`;
                    html += `<td>${s.notifyCount}</td>`;
                    html += `<td>${s.lastNotifyAt ? formatTimestamp(s.lastNotifyAt) : '-'}</td>`;
                    html += '</tr>';
                });
                html += '</tbody></table>';
            }

            html += '<h3 class="intel-section-title">Bindings (' + bindings.length + ')</h3>';
            if (bindings.length === 0) {
                html += '<div class="empty-state">No bindings found.</div>';
            } else {
                html += '<table class="intel-table"><thead><tr>';
                html += '<th>Status</th><th>Feature</th><th>Client</th><th>Server</th><th>Created</th>';
                html += '</tr></thead><tbody>';
                bindings.forEach(b => {
                    let status = b.active
                        ? '<span class="sub-status sub-active">Active</span>'
                        : '<span class="sub-status sub-removed">Removed</span>';
                    html += '<tr>';
                    html += `<td>${status}</td>`;
                    html += `<td>${b.serverFeatureType ? escapeHtml(b.serverFeatureType) : '-'}</td>`;
                    html += `<td>${formatDeviceFeature(b.clientDevice, b.clientFeature)}</td>`;
                    html += `<td>${formatDeviceFeature(b.serverDevice, b.serverFeature)}</td>`;
                    html += `<td>${formatTimestamp(b.createdAt)}</td>`;
                    html += '</tr>';
                });
                html += '</tbody></table>';
            }

            container.innerHTML = html;
        }).catch(err => {
            document.getElementById('panel-subscriptions').innerHTML =
                `<div class="empty-state">Error: ${escapeHtml(err.message)}</div>`;
        });
    }

    function loadMetrics() {
        apiFetch(`/api/traces/${TRACE_ID}/metrics`)
            .then(data => {
                const summary = document.getElementById('metrics-summary');
                let html = '';

                if (!data.heartbeatJitter || data.heartbeatJitter.length === 0) {
                    summary.innerHTML = '<div class="empty-state">No heartbeat data found in this trace.</div>';
                    return;
                }

                html += '<div class="intel-metrics-summary">';
                html += `<div class="intel-stat"><strong>${data.heartbeatJitter.length}</strong> device pair(s)</div>`;
                html += '</div>';

                html += '<h3 class="intel-section-title">Heartbeat Jitter</h3>';
                html += '<table class="intel-table"><thead><tr>';
                html += '<th>Device Pair</th><th>Mean Interval</th><th>Std Dev</th><th>Min</th><th>Max</th><th>Samples</th>';
                html += '</tr></thead><tbody>';
                data.heartbeatJitter.forEach(j => {
                    html += '<tr>';
                    html += `<td>${escapeHtml(j.devicePair)}</td>`;
                    html += `<td>${(j.meanIntervalMs / 1000).toFixed(1)}s</td>`;
                    html += `<td>${(j.stdDevMs / 1000).toFixed(1)}s</td>`;
                    html += `<td>${(j.minIntervalMs / 1000).toFixed(1)}s</td>`;
                    html += `<td>${(j.maxIntervalMs / 1000).toFixed(1)}s</td>`;
                    html += `<td>${j.sampleCount}</td>`;
                    html += '</tr>';
                });
                html += '</tbody></table>';

                html += '<div style="margin-top: 1rem;">';
                html += `<a href="/api/traces/${TRACE_ID}/metrics/export?format=csv" class="btn btn-sm">Export CSV</a> `;
                html += `<a href="/api/traces/${TRACE_ID}/metrics/export?format=json" class="btn btn-sm">Export JSON</a>`;
                html += '</div>';

                summary.innerHTML = html;
            })
            .catch(err => {
                document.getElementById('metrics-summary').innerHTML =
                    `<div class="empty-state">Error: ${escapeHtml(err.message)}</div>`;
            });
    }

    function formatTimestamp(ts) {
        if (!ts) return '-';
        const d = new Date(ts);
        return d.toLocaleTimeString('en-US', { hour12: false, fractionalSecondDigits: 3 });
    }

    function formatDeviceFeature(device, feature) {
        if (!device) return '-';
        // Shorten long device addresses: "d:_i:37916_CEM-400000270" → "CEM-400000270"
        let short = device;
        const underscoreIdx = device.lastIndexOf('_');
        if (underscoreIdx >= 0 && underscoreIdx < device.length - 1) {
            short = device.substring(underscoreIdx + 1);
        }
        let label = escapeHtml(short);
        if (feature) {
            label += ' <span class="feature-addr">[' + escapeHtml(feature) + ']</span>';
        }
        return label;
    }
})();

(function() {
    'use strict';

    const TRACE_ID = window.TRACE_ID;
    const apiFetch = EEBusTracer.apiFetch;
    const escapeHtml = EEBusTracer.escapeHtml;

    let depGraphLoaded = false;
    let writeTrackingLoaded = false;
    let lifecycleLoaded = false;

    // Tab switching
    document.querySelectorAll('.intel-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.intel-tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.intel-panel').forEach(p => p.classList.remove('active'));
            tab.classList.add('active');
            document.getElementById('panel-' + tab.dataset.tab).classList.add('active');

            // Lazy-load dependency tree on first click
            if (tab.dataset.tab === 'depgraph' && !depGraphLoaded) {
                depGraphLoaded = true;
                loadDepGraph();
            }

            // Lazy-load write tracking on first click
            if (tab.dataset.tab === 'writetracking' && !writeTrackingLoaded) {
                writeTrackingLoaded = true;
                loadWriteTracking();
            }

            // Lazy-load lifecycle on first click
            if (tab.dataset.tab === 'lifecycle' && !lifecycleLoaded) {
                lifecycleLoaded = true;
                loadLifecycle();
            }
        });
    });

    // Load all tabs (except depgraph, which is lazy)
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

    // --- Dependency Tree ---

    function loadDepGraph() {
        const container = document.getElementById('depgraph-content');
        container.innerHTML = '<div class="empty-state">Loading dependencies...</div>';

        apiFetch(`/api/traces/${TRACE_ID}/depgraph`)
            .then(data => {
                if ((!data.devices || data.devices.length === 0) && (!data.edges || data.edges.length === 0)) {
                    container.innerHTML = '<div class="empty-state">No dependency data available. Ensure the trace contains device discovery and use case data.</div>';
                    return;
                }

                container.innerHTML = '';
                container.className = '';
                renderTree(container, data);
            })
            .catch(err => {
                container.innerHTML = `<div class="empty-state">Error loading dependencies: ${escapeHtml(err.message)}</div>`;
            });
    }

    function renderTree(container, data) {
        if (!data.devices || data.devices.length === 0) {
            container.innerHTML = '<div class="empty-state">No devices found.</div>';
            return;
        }

        let html = '<div class="intel-grid">';

        data.devices.forEach(dev => {
            html += '<div class="intel-card">';
            html += `<div class="intel-card-header">`;
            html += `<span class="intel-card-title">${escapeHtml(dev.shortName || dev.deviceAddr)}</span>`;
            html += '</div>';
            html += '<div class="intel-card-body">';

            if (!dev.entities || dev.entities.length === 0) {
                html += '<div class="deptree-empty">No discovery data</div>';
            } else {
                dev.entities.forEach(ent => {
                    html += '<div class="deptree-entity">';
                    html += `<span class="entity-node-label">${escapeHtml(ent.entityType)}</span>`;
                    html += ` <span class="feature-addr">${escapeHtml(ent.address)}</span>`;

                    if (ent.features && ent.features.length > 0) {
                        ent.features.forEach(feat => {
                            html += '<div class="deptree-feature">';
                            html += `<span class="deptree-feature-name">${escapeHtml(feat.featureType)}</span>`;
                            html += ` <span class="feature-role">(${escapeHtml(feat.role)})</span>`;

                            if (feat.useCases && feat.useCases.length > 0) {
                                html += '<span class="deptree-uc-list">';
                                feat.useCases.forEach(uc => {
                                    html += `<span class="uc-pill uc-available">${escapeHtml(uc)}</span>`;
                                });
                                html += '</span>';
                            }

                            html += '</div>';
                        });
                    }

                    html += '</div>';
                });
            }

            // Edges related to this device
            const deviceEdges = (data.edges || []).filter(e =>
                e.clientDevice === dev.deviceAddr || e.serverDevice === dev.deviceAddr
            );

            if (deviceEdges.length > 0) {
                const subs = deviceEdges.filter(e => e.type === 'subscription');
                const binds = deviceEdges.filter(e => e.type === 'binding');

                html += '<div class="deptree-edges">';

                if (subs.length > 0) {
                    html += `<div class="deptree-edges-header">Subscriptions (${subs.length})</div>`;
                    subs.forEach(e => {
                        html += renderEdgeRow(e);
                    });
                }

                if (binds.length > 0) {
                    html += `<div class="deptree-edges-header">Bindings (${binds.length})</div>`;
                    binds.forEach(e => {
                        html += renderEdgeRow(e);
                    });
                }

                html += '</div>';
            }

            html += '</div></div>';
        });

        html += '</div>';
        container.innerHTML = html;
    }

    function renderEdgeRow(edge) {
        const statusCls = edge.active ? 'sub-active' : 'sub-removed';
        const statusLabel = edge.active ? 'active' : 'removed';
        return '<div class="deptree-edge-row">' +
            formatDeviceFeature(edge.clientDevice, edge.clientFeature) +
            ' <span class="deptree-arrow">\u2192</span> ' +
            formatDeviceFeature(edge.serverDevice, edge.serverFeature) +
            ` <span class="sub-status ${statusCls}">${statusLabel}</span>` +
            '</div>';
    }

    // --- Write Tracking ---

    function loadWriteTracking() {
        const container = document.getElementById('panel-writetracking');
        container.innerHTML = '<div class="empty-state">Loading write tracking...</div>';

        apiFetch(`/api/traces/${TRACE_ID}/writetracking`)
            .then(data => {
                if ((!data.writes || data.writes.length === 0) && (!data.effectiveState || data.effectiveState.length === 0)) {
                    container.innerHTML = '<div class="empty-state">No write operations found in this trace. Writes to LoadControlLimitListData and SetpointListData will appear here.</div>';
                    return;
                }
                container.innerHTML = renderWriteTracking(data);
            })
            .catch(err => {
                container.innerHTML = `<div class="empty-state">Error loading write tracking: ${escapeHtml(err.message)}</div>`;
            });
    }

    function renderWriteTracking(data) {
        let html = '';

        // Effective State
        if (data.effectiveState && data.effectiveState.length > 0) {
            html += '<h3 class="intel-section-title">Effective State</h3>';
            html += '<div class="intel-grid">';
            data.effectiveState.forEach(s => {
                html += '<div class="intel-card">';
                html += '<div class="intel-card-header">';
                html += `<span class="intel-card-title">${escapeHtml(s.label)}</span>`;
                html += `<span class="sub-status ${resultStatusClass(s.result)}">${escapeHtml(s.result)}</span>`;
                html += '</div>';
                html += '<div class="intel-card-body">';
                html += `<div class="intel-stat"><strong>${s.value}</strong>`;
                if (s.unit) html += ` ${escapeHtml(s.unit)}`;
                html += '</div>';
                if (s.isActive !== undefined && s.isActive !== null) {
                    const activeCls = s.isActive ? 'sub-active' : 'sub-removed';
                    const activeLabel = s.isActive ? 'active' : 'inactive';
                    html += `<span class="sub-status ${activeCls}">${activeLabel}</span> `;
                }
                html += `<div style="margin-top:4px;font-size:12px;color:var(--text-muted)">Since ${formatTimestamp(s.since)}</div>`;
                html += '</div></div>';
            });
            html += '</div>';
        }

        // Write History
        if (data.writes && data.writes.length > 0) {
            html += '<h3 class="intel-section-title">Write History (' + data.writes.length + ')</h3>';
            html += '<table class="intel-table"><thead><tr>';
            html += '<th>Time</th><th>Direction</th><th>Type</th><th>ID / Label</th><th>Value</th><th>Active</th><th>Result</th><th>Latency</th><th>Duration</th>';
            html += '</tr></thead><tbody>';
            data.writes.forEach(w => {
                html += '<tr>';
                html += `<td>${formatTimestamp(w.timestamp)}</td>`;
                html += `<td>${formatDeviceFeature(w.source)} \u2192 ${formatDeviceFeature(w.dest)}</td>`;
                html += `<td>${escapeHtml(w.dataType)}</td>`;
                html += `<td>${escapeHtml(w.label)} <span class="feature-addr">[${escapeHtml(w.itemId)}]</span></td>`;
                html += `<td><strong>${w.value}</strong>${w.unit ? ' ' + escapeHtml(w.unit) : ''}</td>`;

                if (w.isActive !== undefined && w.isActive !== null) {
                    const activeCls = w.isActive ? 'sub-active' : 'sub-removed';
                    const activeLabel = w.isActive ? 'yes' : 'no';
                    html += `<td><span class="sub-status ${activeCls}">${activeLabel}</span></td>`;
                } else {
                    html += '<td>-</td>';
                }

                html += `<td><span class="sub-status ${resultStatusClass(w.result)}">${escapeHtml(w.result)}</span></td>`;
                html += `<td>${w.latencyMs != null ? w.latencyMs.toFixed(1) + ' ms' : '-'}</td>`;
                html += `<td>${w.durationMs != null ? formatDuration(w.durationMs) : '-'}</td>`;
                html += '</tr>';
            });
            html += '</tbody></table>';
        }

        return html;
    }

    function resultStatusClass(result) {
        switch (result) {
            case 'accepted': return 'sub-active';
            case 'rejected': return 'sub-rejected';
            case 'pending': return 'sub-stale';
            default: return 'sub-removed';
        }
    }

    function formatDuration(ms) {
        if (ms < 1000) return ms.toFixed(0) + ' ms';
        if (ms < 60000) return (ms / 1000).toFixed(1) + ' s';
        return (ms / 60000).toFixed(1) + ' min';
    }

    // --- Lifecycle ---

    function loadLifecycle() {
        const container = document.getElementById('panel-lifecycle');
        container.innerHTML = '<div class="empty-state">Loading lifecycle checklist...</div>';

        apiFetch(`/api/traces/${TRACE_ID}/lifecycle`)
            .then(data => {
                if (!data || data.length === 0) {
                    container.innerHTML = '<div class="empty-state">No use cases detected in this trace.</div>';
                    return;
                }
                container.innerHTML = renderLifecycle(data);
            })
            .catch(err => {
                container.innerHTML = `<div class="empty-state">Error loading lifecycle: ${escapeHtml(err.message)}</div>`;
            });
    }

    function renderLifecycle(data) {
        // Group by device
        var devices = {};
        var deviceOrder = [];
        data.forEach(function(lc) {
            if (!devices[lc.deviceAddr]) {
                devices[lc.deviceAddr] = { shortName: lc.shortName || lc.deviceAddr, ucs: [] };
                deviceOrder.push(lc.deviceAddr);
            }
            devices[lc.deviceAddr].ucs.push(lc);
        });

        var html = '<div class="intel-grid lc-grid">';

        deviceOrder.forEach(function(addr) {
            var dev = devices[addr];
            html += '<div class="intel-card">';
            html += '<div class="intel-card-header">';
            html += '<span class="intel-card-title">' + escapeHtml(dev.shortName) + '</span>';
            html += '</div>';
            html += '<div class="intel-card-body lc-card-body">';

            dev.ucs.forEach(function(lc) {
                var overallCls = lifecycleStatusClass(lc.overallStatus);
                var overallLabel = lifecycleStatusLabel(lc.overallStatus);
                var expanded = lc.overallStatus !== 'pass' && lc.overallStatus !== 'na';

                html += '<div class="lc-uc' + (expanded ? ' open' : '') + '">';
                html += '<div class="lc-uc-header" onclick="this.parentElement.classList.toggle(\'open\')">';
                html += '<span class="lc-chevron">\u25B6</span>';
                html += '<span class="uc-pill ' + (lc.available ? 'uc-available' : 'uc-unavailable') + '">' + escapeHtml(lc.useCaseAbbr) + '</span>';
                html += '<span class="sub-status ' + overallCls + '">' + overallLabel + '</span>';

                // Step status dots
                html += '<span class="lc-dots">';
                lc.steps.forEach(function(step) {
                    var dotCls = lifecycleStatusClass(step.status);
                    html += '<span class="lc-dot ' + dotCls + '" title="' + escapeHtml(step.name) + ': ' + escapeHtml(step.details) + '"></span>';
                });
                html += '</span>';
                html += '</div>';

                // Collapsible step list
                html += '<div class="lc-uc-body">';
                lc.steps.forEach(function(step) {
                    var icon = lifecycleStepIcon(step.status);
                    var iconCls = 'lc-step-icon lc-step-icon-' + step.status;
                    html += '<div class="lc-step">';
                    html += '<span class="' + iconCls + '">' + icon + '</span>';
                    html += '<span class="lc-step-name">' + escapeHtml(step.name) + '</span>';
                    if (step.status !== 'pass' && step.status !== 'na' && step.details) {
                        html += '<div class="lc-step-details">' + formatStepDetails(step.details) + '</div>';
                    }
                    html += '</div>';
                });
                html += '</div>';

                html += '</div>';
            });

            html += '</div></div>';
        });

        html += '</div>';
        return html;
    }

    function lifecycleStepIcon(status) {
        switch (status) {
            case 'pass': return '\u2713';
            case 'fail': return '\u2717';
            case 'partial': return '\u25D1';
            case 'pending': return '\u25CB';
            case 'na': return '\u2014';
            default: return '\u25CB';
        }
    }

    function formatStepDetails(details) {
        if (!details) return '';
        var sep = details.indexOf('; missing: ');
        if (sep === -1) return escapeHtml(details);
        var summary = details.substring(0, sep);
        var missing = details.substring(sep + 11); // skip '; missing: '
        var items = missing.split(', ');
        var html = escapeHtml(summary);
        html += '<div class="lc-missing" onclick="this.classList.toggle(\'open\')">';
        html += '<span class="lc-missing-toggle">\u25B6</span> ';
        html += items.length + ' missing';
        html += '<ul class="lc-missing-list">';
        for (var i = 0; i < items.length; i++) {
            html += '<li>' + escapeHtml(items[i]) + '</li>';
        }
        html += '</ul></div>';
        return html;
    }

    function lifecycleStatusClass(status) {
        switch (status) {
            case 'pass': return 'sub-active';
            case 'fail': return 'sub-rejected';
            case 'partial': return 'sub-stale';
            case 'pending': return 'sub-pending';
            case 'na': return 'sub-removed';
            default: return 'sub-removed';
        }
    }

    function lifecycleStatusLabel(status) {
        switch (status) {
            case 'pass': return 'Pass';
            case 'fail': return 'Fail';
            case 'partial': return 'Partial';
            case 'pending': return 'Pending';
            case 'na': return 'N/A';
            default: return status;
        }
    }

    // --- Helpers ---

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

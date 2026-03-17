// EEBusTracer Charts — Dynamic chart selector with custom chart builder
// Uses Chart.js for line/step charts

(function() {
    'use strict';

    var TRACE_ID = window.TRACE_ID;
    if (!TRACE_ID) return;

    var chartSelector = document.getElementById('chart-selector');
    var newBtn = document.getElementById('chart-new-btn');
    var deleteBtn = document.getElementById('chart-delete-btn');
    var seriesToggles = document.getElementById('chart-series-toggles');
    var overlayCheck = document.getElementById('chart-overlay');
    var resetBtn = document.getElementById('chart-reset');
    var exportBtn = document.getElementById('chart-export-csv');
    var chartCanvas = document.getElementById('chart-main');

    // Builder elements
    var builderOverlay = document.getElementById('chart-builder-overlay');
    var builderName = document.getElementById('builder-name');
    var builderSources = document.getElementById('builder-sources');
    var builderCancel = document.getElementById('builder-cancel');
    var builderSave = document.getElementById('builder-save');
    var builderTypeBtns = document.querySelectorAll('.builder-type-btn');

    var chartInstance = null;
    var currentData = null;
    var chartDefinitions = [];
    var selectedChartId = null;
    var selectedChartType = 'line'; // for builder
    var discoveredSources = [];

    function cssVar(name) {
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    }

    function getChartColors() {
        var colors = [];
        for (var i = 1; i <= 10; i++) {
            colors.push(cssVar('--chart-' + i));
        }
        return colors;
    }

    var CHART_COLORS = getChartColors();

    function init() {
        loadChartDefinitions();
    }

    function loadChartDefinitions() {
        EEBusTracer.apiFetch('/api/traces/' + TRACE_ID + '/charts')
            .then(function(charts) {
                chartDefinitions = charts || [];
                populateSelector();
                if (chartDefinitions.length > 0) {
                    selectedChartId = chartDefinitions[0].id;
                    chartSelector.value = selectedChartId;
                    loadChartData();
                }
            })
            .catch(function(err) {
                document.getElementById('chart-canvas').innerHTML =
                    '<div class="empty-state">Failed to load charts: ' + err.message + '</div>';
            });
    }

    function populateSelector() {
        chartSelector.innerHTML = '';
        chartDefinitions.forEach(function(cd) {
            var opt = document.createElement('option');
            opt.value = cd.id;
            opt.textContent = cd.name + (cd.isBuiltIn ? '' : ' *');
            chartSelector.appendChild(opt);
        });
    }

    function loadChartData() {
        if (!selectedChartId) return;

        var cd = chartDefinitions.find(function(c) { return c.id === selectedChartId; });
        updateDeleteButton(cd);

        EEBusTracer.apiFetch('/api/traces/' + TRACE_ID + '/charts/' + selectedChartId + '/data')
            .then(function(data) {
                currentData = data;
                // Override type from chart definition for rendering
                if (cd) {
                    currentData._chartType = cd.chartType;
                }
                buildSeriesToggles(data.series);
                renderChart(data);
            })
            .catch(function(err) {
                document.getElementById('chart-canvas').innerHTML =
                    '<div class="empty-state">Failed to load data: ' + err.message + '</div>';
            });
    }

    function updateDeleteButton(cd) {
        if (cd && !cd.isBuiltIn) {
            deleteBtn.style.display = '';
        } else {
            deleteBtn.style.display = 'none';
        }
    }

    function buildSeriesToggles(series) {
        if (!seriesToggles) return;
        seriesToggles.innerHTML = '';

        if (!series) return;
        series.forEach(function(s, i) {
            var label = document.createElement('label');
            label.className = 'chart-series-toggle';

            var cb = document.createElement('input');
            cb.type = 'checkbox';
            cb.checked = true;
            cb.dataset.seriesIdx = i;
            cb.addEventListener('change', function() {
                if (chartInstance) {
                    chartInstance.data.datasets[i].hidden = !cb.checked;
                    chartInstance.update();
                }
            });

            var dot = document.createElement('span');
            dot.style.display = 'inline-block';
            dot.style.width = '8px';
            dot.style.height = '8px';
            dot.style.borderRadius = '50%';
            dot.style.background = CHART_COLORS[i % CHART_COLORS.length];

            var text = s.label;
            if (s.unit) text += ' (' + s.unit + ')';

            label.appendChild(cb);
            label.appendChild(dot);
            label.appendChild(document.createTextNode(' ' + text));
            seriesToggles.appendChild(label);
        });
    }

    function renderChart(data) {
        if (chartInstance) {
            chartInstance.destroy();
            chartInstance = null;
        }

        if (!data.series || data.series.length === 0) {
            document.getElementById('chart-canvas').innerHTML =
                '<div class="empty-state">No time series data found for this trace</div>';
            return;
        }

        // Ensure canvas is present
        var canvasContainer = document.getElementById('chart-canvas');
        if (!canvasContainer.querySelector('canvas')) {
            canvasContainer.innerHTML = '<canvas id="chart-main"></canvas>';
            chartCanvas = document.getElementById('chart-main');
        }

        var isStepped = data._chartType === 'step' || data.type === 'step';

        // Collect distinct units and map each to a y-axis ID
        var units = [];
        var unitToAxisId = {};
        data.series.forEach(function(s) {
            var u = s.unit || '';
            if (unitToAxisId[u] === undefined) {
                var axisId = units.length === 0 ? 'y' : 'y' + units.length;
                unitToAxisId[u] = axisId;
                units.push(u);
            }
        });

        var datasets = data.series.map(function(s, i) {
            return {
                label: s.label,
                data: s.dataPoints.map(function(dp) {
                    return { x: new Date(dp.timestamp), y: dp.value };
                }),
                borderColor: CHART_COLORS[i % CHART_COLORS.length],
                backgroundColor: CHART_COLORS[i % CHART_COLORS.length] + '33',
                borderWidth: 2,
                pointRadius: 3,
                pointHoverRadius: 5,
                fill: false,
                tension: 0.1,
                stepped: isStepped ? 'before' : false,
                yAxisID: unitToAxisId[s.unit || ''],
                _unit: s.unit || ''
            };
        });

        var ctx = chartCanvas.getContext('2d');
        chartInstance = new Chart(ctx, {
            type: 'line',
            data: { datasets: datasets },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: {
                    mode: 'nearest',
                    intersect: false
                },
                scales: (function() {
                    var scales = {
                        x: {
                            type: 'time',
                            time: {
                                tooltipFormat: 'HH:mm:ss.SSS',
                                displayFormats: {
                                    millisecond: 'HH:mm:ss.SSS',
                                    second: 'HH:mm:ss',
                                    minute: 'HH:mm'
                                }
                            },
                            grid: {
                                color: cssVar('--chart-grid')
                            },
                            ticks: {
                                color: cssVar('--chart-tick'),
                                font: { family: "'SF Mono', monospace", size: 10 }
                            }
                        }
                    };
                    units.forEach(function(u, idx) {
                        var axisId = idx === 0 ? 'y' : 'y' + idx;
                        scales[axisId] = {
                            position: idx === 0 ? 'left' : 'right',
                            grid: {
                                drawOnChartArea: idx === 0,
                                color: cssVar('--chart-grid')
                            },
                            ticks: {
                                color: cssVar('--chart-tick'),
                                font: { family: "'SF Mono', monospace", size: 10 }
                            }
                        };
                        if (u) {
                            scales[axisId].title = {
                                display: true,
                                text: u,
                                color: cssVar('--chart-tick'),
                                font: { family: "'SF Mono', monospace", size: 11 }
                            };
                        }
                    });
                    return scales;
                })(),
                plugins: {
                    legend: {
                        display: false
                    },
                    tooltip: {
                        backgroundColor: cssVar('--chart-tooltip-bg'),
                        borderColor: cssVar('--chart-tooltip-border'),
                        borderWidth: 1,
                        titleColor: cssVar('--chart-tooltip-title'),
                        bodyColor: cssVar('--chart-tooltip-body'),
                        titleFont: { family: "'SF Mono', monospace", size: 11 },
                        bodyFont: { family: "'SF Mono', monospace", size: 11 },
                        callbacks: {
                            label: function(context) {
                                var label = context.dataset.label || '';
                                var val = context.parsed.y;
                                var unit = context.dataset._unit || '';
                                if (val !== null && val !== undefined) {
                                    label += ': ' + val;
                                    if (unit) label += ' ' + unit;
                                }
                                return label;
                            }
                        }
                    },
                    zoom: {
                        pan: {
                            enabled: true,
                            mode: 'x'
                        },
                        zoom: {
                            wheel: { enabled: true },
                            pinch: { enabled: true },
                            mode: 'x'
                        }
                    }
                }
            }
        });
    }

    // --- Builder ---

    function openBuilder() {
        builderName.value = '';
        selectedChartType = 'line';
        builderTypeBtns.forEach(function(btn) {
            btn.classList.toggle('active', btn.dataset.type === 'line');
        });
        builderOverlay.style.display = 'flex';
        builderSources.innerHTML = '<div class="empty-state">Scanning trace...</div>';

        EEBusTracer.apiFetch('/api/traces/' + TRACE_ID + '/timeseries/discover')
            .then(function(result) {
                discoveredSources = result.sources || [];
                renderBuilderSources();
            })
            .catch(function() {
                builderSources.innerHTML = '<div class="empty-state">Failed to scan trace</div>';
            });
    }

    function renderBuilderSources() {
        builderSources.innerHTML = '';
        if (discoveredSources.length === 0) {
            builderSources.innerHTML = '<div class="empty-state">No chartable data found in this trace</div>';
            return;
        }

        discoveredSources.forEach(function(src, i) {
            var item = document.createElement('label');
            item.className = 'chart-builder-source';

            var cb = document.createElement('input');
            cb.type = 'checkbox';
            cb.checked = true;
            cb.dataset.sourceIdx = i;

            var info = document.createElement('span');
            info.textContent = src.functionSet + ' (' + src.messageCount + ' msgs, IDs: ' + src.sampleIds.join(', ') + ')';

            item.appendChild(cb);
            item.appendChild(info);
            builderSources.appendChild(item);
        });
    }

    function closeBuilder() {
        builderOverlay.style.display = 'none';
    }

    function saveChart() {
        var name = builderName.value.trim();
        if (!name) {
            builderName.focus();
            return;
        }

        var sources = [];
        var checkboxes = builderSources.querySelectorAll('input[type="checkbox"]');
        checkboxes.forEach(function(cb) {
            if (cb.checked) {
                var idx = parseInt(cb.dataset.sourceIdx, 10);
                var src = discoveredSources[idx];
                sources.push({
                    functionSet: src.functionSet,
                    cmdKey: src.cmdKey,
                    dataArrayKey: src.dataArrayKey,
                    idField: src.idField,
                    classifiers: ['reply', 'notify', 'write']
                });
            }
        });

        if (sources.length === 0) return;

        var body = JSON.stringify({
            name: name,
            chartType: selectedChartType,
            sources: JSON.stringify(sources)
        });

        fetch('/api/traces/' + TRACE_ID + '/charts', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: body
        })
        .then(function(resp) { return resp.json(); })
        .then(function(created) {
            closeBuilder();
            // Reload charts and select the new one
            EEBusTracer.apiFetch('/api/traces/' + TRACE_ID + '/charts')
                .then(function(charts) {
                    chartDefinitions = charts || [];
                    populateSelector();
                    selectedChartId = created.id;
                    chartSelector.value = selectedChartId;
                    loadChartData();
                });
        });
    }

    function deleteChart() {
        if (!selectedChartId) return;
        var cd = chartDefinitions.find(function(c) { return c.id === selectedChartId; });
        if (!cd || cd.isBuiltIn) return;

        if (!confirm('Delete chart "' + cd.name + '"?')) return;

        fetch('/api/charts/' + selectedChartId, { method: 'DELETE' })
            .then(function() {
                loadChartDefinitions();
            });
    }

    // --- Event listeners ---

    if (chartSelector) {
        chartSelector.addEventListener('change', function() {
            selectedChartId = parseInt(chartSelector.value, 10);
            loadChartData();
        });
    }

    if (newBtn) {
        newBtn.addEventListener('click', openBuilder);
    }

    if (deleteBtn) {
        deleteBtn.addEventListener('click', deleteChart);
    }

    if (builderCancel) {
        builderCancel.addEventListener('click', closeBuilder);
    }

    if (builderSave) {
        builderSave.addEventListener('click', saveChart);
    }

    builderTypeBtns.forEach(function(btn) {
        btn.addEventListener('click', function() {
            selectedChartType = btn.dataset.type;
            builderTypeBtns.forEach(function(b) {
                b.classList.toggle('active', b === btn);
            });
        });
    });

    if (builderOverlay) {
        builderOverlay.addEventListener('click', function(e) {
            if (e.target === builderOverlay) closeBuilder();
        });
    }

    if (overlayCheck) {
        overlayCheck.addEventListener('change', function() {
            if (!currentData || !chartInstance || !selectedChartId) return;
            if (overlayCheck.checked) {
                // Find the "other" chart to overlay
                var currentCd = chartDefinitions.find(function(c) { return c.id === selectedChartId; });
                var otherCd = chartDefinitions.find(function(c) {
                    return c.id !== selectedChartId && c.isBuiltIn;
                });
                if (!otherCd) return;

                EEBusTracer.apiFetch('/api/traces/' + TRACE_ID + '/charts/' + otherCd.id + '/data')
                    .then(function(otherData) {
                        var combined = {
                            type: 'overlay',
                            _chartType: currentCd ? currentCd.chartType : 'line',
                            series: (currentData.series || []).concat(otherData.series || [])
                        };
                        buildSeriesToggles(combined.series);
                        renderChart(combined);
                    });
            } else {
                loadChartData();
            }
        });
    }

    if (resetBtn) {
        resetBtn.addEventListener('click', function() {
            if (chartInstance) {
                chartInstance.resetZoom();
            }
        });
    }

    if (exportBtn) {
        exportBtn.addEventListener('click', function() {
            if (!currentData || !currentData.series || currentData.series.length === 0) return;

            var rows = ['timestamp'];
            currentData.series.forEach(function(s) {
                rows[0] += ',' + s.label.replace(/,/g, ';');
            });
            rows[0] += '\n';

            var tsMap = {};
            currentData.series.forEach(function(s, si) {
                s.dataPoints.forEach(function(dp) {
                    var key = dp.timestamp;
                    if (!tsMap[key]) {
                        tsMap[key] = {};
                    }
                    tsMap[key][si] = dp.value;
                });
            });

            var timestamps = Object.keys(tsMap).sort();
            timestamps.forEach(function(ts) {
                var row = ts;
                currentData.series.forEach(function(s, si) {
                    row += ',' + (tsMap[ts][si] !== undefined ? tsMap[ts][si] : '');
                });
                rows.push(row + '\n');
            });

            var blob = new Blob(rows, { type: 'text/csv;charset=utf-8' });
            var url = URL.createObjectURL(blob);
            var a = document.createElement('a');
            a.href = url;
            a.download = 'timeseries_' + TRACE_ID + '.csv';
            a.click();
            URL.revokeObjectURL(url);
        });
    }

    // Re-render chart on theme change
    document.addEventListener('theme-changed', function() {
        CHART_COLORS = getChartColors();
        if (currentData) {
            buildSeriesToggles(currentData.series);
            renderChart(currentData);
        }
    });

    init();
})();

// EEBusTracer mDNS Discovery Page

(function() {
    'use strict';

    var escapeHtml = window.EEBusTracer ? EEBusTracer.escapeHtml : function(t) { return t; };

    const btnStart = document.getElementById('btn-mdns-start');
    const btnStop = document.getElementById('btn-mdns-stop');
    const statusEl = document.getElementById('mdns-status');
    const devicesEl = document.getElementById('discovery-devices');

    let ws = null;
    let devices = {};

    // Load initial state
    loadStatus();
    loadDevices();

    async function loadStatus() {
        try {
            const resp = await fetch('/api/mdns/status');
            const data = await resp.json();
            if (data.running) {
                btnStart.disabled = true;
                btnStop.disabled = false;
                statusEl.textContent = 'Scanning... (' + data.deviceCount + ' devices)';
                connectWebSocket();
            }
        } catch (e) {
            console.error('Load mDNS status failed:', e);
        }
    }

    async function loadDevices() {
        try {
            const resp = await fetch('/api/mdns/devices');
            const data = await resp.json();
            devices = {};
            if (data && data.length > 0) {
                data.forEach(function(d) {
                    devices[d.instanceName] = d;
                });
            }
            renderDevices();
        } catch (e) {
            console.error('Load mDNS devices failed:', e);
        }
    }

    if (btnStart) {
        btnStart.addEventListener('click', async function() {
            try {
                const resp = await fetch('/api/mdns/start', {method: 'POST'});
                if (resp.ok) {
                    btnStart.disabled = true;
                    btnStop.disabled = false;
                    statusEl.textContent = 'Scanning...';
                    connectWebSocket();
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
        btnStop.addEventListener('click', async function() {
            try {
                const resp = await fetch('/api/mdns/stop', {method: 'POST'});
                if (resp.ok) {
                    btnStart.disabled = false;
                    btnStop.disabled = true;
                    statusEl.textContent = 'Stopped';
                    if (ws) { ws.close(); ws = null; }
                }
            } catch (e) {
                alert('Stop failed: ' + e.message);
            }
        });
    }

    function connectWebSocket() {
        if (ws) return;
        // Connect to a generic WS — mDNS events come via the hub.
        // Use trace ID 0 as a sentinel for non-trace events.
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(proto + '//' + location.host + '/api/traces/0/live');

        ws.onmessage = function(event) {
            try {
                const data = JSON.parse(event.data);
                if (data.type === 'mdns_device') {
                    updateDiscoveryPanel(data.payload);
                }
            } catch (e) {
                // ignore parse errors
            }
        };

        ws.onclose = function() { ws = null; };
    }

    // Exported for use by app.js typed WS events.
    window.updateDiscoveryPanel = function(device) {
        devices[device.instanceName] = device;
        renderDevices();
        // Update status count.
        const count = Object.keys(devices).length;
        if (statusEl && btnStop && !btnStop.disabled) {
            statusEl.textContent = 'Scanning... (' + count + ' devices)';
        }
    };

    function renderDevices() {
        if (!devicesEl) return;

        const keys = Object.keys(devices);
        if (keys.length === 0) {
            devicesEl.innerHTML = '<div class="empty-state">No devices discovered yet</div>';
            return;
        }

        let html = '';
        keys.sort().forEach(function(key) {
            const d = devices[key];
            const onlineClass = d.online ? 'online' : 'offline';
            const addrs = d.addresses ? d.addresses.join(', ') : 'unknown';
            const connectAddr = d.addresses && d.addresses.length > 0 ? d.addresses[0] : '';

            html += '<div class="discovery-card">';
            html += '<div class="discovery-card-header">';
            html += '<span class="discovery-name">' + escapeHtml(d.instanceName) + '</span>';
            html += '<span class="discovery-badge ' + onlineClass + '">' + (d.online ? 'Online' : 'Offline') + '</span>';
            html += '</div>';
            html += '<div class="discovery-card-body">';
            if (d.brand || d.model) {
                html += '<div class="discovery-field"><span class="discovery-label">Device:</span> ' + escapeHtml((d.brand || '') + ' ' + (d.model || '')) + '</div>';
            }
            if (d.deviceType) {
                html += '<div class="discovery-field"><span class="discovery-label">Type:</span> ' + escapeHtml(d.deviceType) + '</div>';
            }
            html += '<div class="discovery-field"><span class="discovery-label">Address:</span> ' + escapeHtml(addrs) + ':' + d.port + '</div>';
            if (d.hostName) {
                html += '<div class="discovery-field"><span class="discovery-label">Host:</span> ' + escapeHtml(d.hostName) + '</div>';
            }
            if (d.ski) {
                html += '<div class="discovery-field"><span class="discovery-label">SKI:</span> <code>' + escapeHtml(d.ski) + '</code></div>';
            }
            if (d.identifier) {
                html += '<div class="discovery-field"><span class="discovery-label">ID:</span> ' + escapeHtml(d.identifier) + '</div>';
            }
            html += '</div>';
            if (connectAddr) {
                html += '<div class="discovery-card-footer">';
                html += '<button class="btn btn-connect" onclick="connectToDevice(\'' + escapeHtml(connectAddr) + '\',' + d.port + ')">Connect</button>';
                html += '</div>';
            }
            html += '</div>';
        });

        devicesEl.innerHTML = html;
    }

    window.connectToDevice = function(host, port) {
        // Set capture target and redirect to main page.
        const hostInput = document.getElementById('target-host');
        const portInput = document.getElementById('target-port');
        const modeSelect = document.getElementById('capture-mode');
        if (hostInput) hostInput.value = host;
        if (portInput) portInput.value = port;
        if (modeSelect) modeSelect.value = 'udp';
        window.location.href = '/';
    };
})();

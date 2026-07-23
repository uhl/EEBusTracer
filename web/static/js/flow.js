// EEBusTracer — Flow View (Sequence Diagram + Swimlane Overview)
// Canvas-based virtual-scroll sequence diagram.

(function() {
    'use strict';

    var ROW_HEIGHT = 36;
    var OVERVIEW_HEIGHT = 48;
    var LANE_MIN_WIDTH = 180;
    var ARROW_HEAD = 8;
    var HEADER_HEIGHT = 32;
    var OVERSCAN = 10;
    var SELF_ARROW_WIDTH = 40;
    var LEFT_MARGIN = 90;

    // Classifier → CSS color variable mapping
    var CLASSIFIER_COLORS = {
        read:   '--badge-read-fg',
        reply:  '--badge-reply-fg',
        write:  '--badge-write-fg',
        call:   '--badge-call-fg',
        notify: '--badge-notify-fg',
        result: '--badge-result-fg'
    };

    function FlowView(opts) {
        this.container = opts.container;
        this.onSelect = opts.onSelect || function() {};
        this.data = [];
        this.participants = [];
        this.correlationMap = {};   // msgCounter → {index, summary}
        this.correlationPairs = []; // [{reqIdx, respIdx}]
        this.laneX = [];            // x position per participant
        this.laneWidth = LANE_MIN_WIDTH;
        this.selectedIndex = -1;
        this.autoScroll = true;
        this._colors = {};
        this._raf = null;
        this._destroyed = false;

        this._createDOM();
        this._readColors();
        this._bindEvents();
    }

    FlowView.prototype._createDOM = function() {
        this.container.innerHTML = '';

        // Overview canvas
        this.overviewWrap = document.createElement('div');
        this.overviewWrap.className = 'flow-overview';
        this.overviewCanvas = document.createElement('canvas');
        this.overviewCanvas.className = 'flow-overview-canvas';
        this.overviewWrap.appendChild(this.overviewCanvas);
        this.container.appendChild(this.overviewWrap);

        // Participant header (sticky)
        this.header = document.createElement('div');
        this.header.className = 'flow-header';
        this.container.appendChild(this.header);

        // Scroll container
        this.scrollEl = document.createElement('div');
        this.scrollEl.className = 'flow-scroll';

        // Diagram canvas in a sticky zero-height wrapper so it stays at
        // the viewport top without consuming scroll height.
        this.canvasWrap = document.createElement('div');
        this.canvasWrap.className = 'flow-canvas-wrap';
        this.canvas = document.createElement('canvas');
        this.canvas.className = 'flow-canvas';
        this.canvasWrap.appendChild(this.canvas);
        this.scrollEl.appendChild(this.canvasWrap);

        // Spacer (sets scrollbar height)
        this.spacer = document.createElement('div');
        this.spacer.className = 'flow-spacer';
        this.scrollEl.appendChild(this.spacer);

        this.container.appendChild(this.scrollEl);
    };

    FlowView.prototype._readColors = function() {
        var style = getComputedStyle(document.documentElement);
        for (var cls in CLASSIFIER_COLORS) {
            this._colors[cls] = style.getPropertyValue(CLASSIFIER_COLORS[cls]).trim() || '#888';
        }
        this._colors['_text'] = style.getPropertyValue('--text-primary').trim() || '#e8e8ed';
        this._colors['_muted'] = style.getPropertyValue('--text-muted').trim() || '#55556a';
        this._colors['_secondary'] = style.getPropertyValue('--text-secondary').trim() || '#8888a0';
        this._colors['_bg'] = style.getPropertyValue('--bg-primary').trim() || '#0c0c0f';
        this._colors['_surface'] = style.getPropertyValue('--bg-surface').trim() || '#1a1a21';
        this._colors['_selected'] = style.getPropertyValue('--row-selected').trim() || 'rgba(212,160,83,0.10)';
        this._colors['_accent'] = style.getPropertyValue('--accent').trim() || '#d4a053';
        this._colors['_border'] = style.getPropertyValue('--border').trim() || 'rgba(255,255,255,0.06)';
        this._colors['_bgSecondary'] = style.getPropertyValue('--bg-secondary').trim() || '#111115';
    };

    FlowView.prototype._bindEvents = function() {
        var self = this;

        // Scroll → render
        this.scrollEl.addEventListener('scroll', function() {
            self._scheduleRender();
        });

        // Click → hit test
        this.canvas.addEventListener('click', function(e) {
            var rect = self.canvas.getBoundingClientRect();
            var y = e.clientY - rect.top;
            var scrollTop = self.scrollEl.scrollTop;
            var idx = Math.floor((scrollTop + y) / ROW_HEIGHT);
            if (idx >= 0 && idx < self.data.length) {
                self.selectedIndex = idx;
                self._render();
                self._renderOverview();
                var item = self.data[idx];
                if (item) self.onSelect(item);
            }
        });

        // Overview click → jump
        this.overviewCanvas.addEventListener('click', function(e) {
            if (self.data.length === 0) return;
            var rect = self.overviewCanvas.getBoundingClientRect();
            var x = e.clientX - rect.left;
            var ratio = x / rect.width;
            var targetIdx = Math.floor(ratio * self.data.length);
            self.scrollToIndex(targetIdx);
        });

        // Overview drag
        var dragging = false;
        this.overviewCanvas.addEventListener('mousedown', function(e) {
            dragging = true;
            e.preventDefault();
        });
        document.addEventListener('mousemove', function(e) {
            if (!dragging || self.data.length === 0) return;
            var rect = self.overviewCanvas.getBoundingClientRect();
            var x = Math.max(0, Math.min(e.clientX - rect.left, rect.width));
            var ratio = x / rect.width;
            var targetIdx = Math.floor(ratio * self.data.length);
            self.scrollToIndex(targetIdx);
        });
        document.addEventListener('mouseup', function() {
            dragging = false;
        });

        // Resize observer
        if (typeof ResizeObserver !== 'undefined') {
            this._resizeObserver = new ResizeObserver(function() {
                if (self._destroyed) return;
                self._layoutParticipants();
                self._updateCanvasSize();
                self._render();
                self._renderOverview();
            });
            this._resizeObserver.observe(this.container);
        }

        // Theme change detection
        this._themeObserver = new MutationObserver(function() {
            self._readColors();
            self._render();
            self._renderOverview();
        });
        this._themeObserver.observe(document.documentElement, {
            attributes: true,
            attributeFilter: ['data-theme']
        });
    };

    FlowView.prototype.setData = function(summaries) {
        this.data = summaries || [];
        this._extractParticipants();
        this._buildCorrelationMap();
        this._layoutParticipants();
        this._updateSpacer();
        this._updateCanvasSize();
        this._renderHeader();
        this._render();
        this._renderOverview();
    };

    FlowView.prototype.appendMessage = function(summary) {
        this.data.push(summary);

        // Update participants if new device appears
        var changed = false;
        var self = this;
        var addrs = effectiveAddrs(summary);
        [addrs.src, addrs.dst].forEach(function(addr) {
            if (!addr) return;
            var found = false;
            for (var i = 0; i < self.participants.length; i++) {
                if (self.participants[i].deviceAddr === addr) {
                    found = true;
                    break;
                }
            }
            if (!found) {
                self.participants.push({deviceAddr: addr, shortName: shortDeviceName(addr)});
                changed = true;
            }
        });

        // Update correlation map
        if (summary.msgCounter) {
            this.correlationMap[summary.msgCounter] = {
                index: this.data.length - 1,
                summary: summary
            };
        }
        if (summary.msgCounterRef && this.correlationMap[summary.msgCounterRef]) {
            this.correlationPairs.push({
                reqIdx: this.correlationMap[summary.msgCounterRef].index,
                respIdx: this.data.length - 1
            });
        }

        if (changed) {
            this._layoutParticipants();
            this._renderHeader();
        }

        this._updateSpacer();

        if (this.autoScroll) {
            this.scrollToIndex(this.data.length - 1);
        }

        this._scheduleRender();
        this._renderOverview();
    };

    FlowView.prototype.setSelectedIndex = function(index) {
        this.selectedIndex = index;
        this._render();
        this._renderOverview();
    };

    FlowView.prototype.scrollToIndex = function(index) {
        if (index < 0 || index >= this.data.length) return;
        var targetTop = index * ROW_HEIGHT;
        var viewH = this.scrollEl.clientHeight;
        // Center the target
        this.scrollEl.scrollTop = targetTop - viewH / 2 + ROW_HEIGHT / 2;
    };

    FlowView.prototype.setAutoScroll = function(enabled) {
        this.autoScroll = enabled;
    };

    FlowView.prototype.destroy = function() {
        this._destroyed = true;
        if (this._resizeObserver) this._resizeObserver.disconnect();
        if (this._themeObserver) this._themeObserver.disconnect();
        if (this._raf) cancelAnimationFrame(this._raf);
        this.container.innerHTML = '';
    };

    // --- Internal ---

    FlowView.prototype._extractParticipants = function() {
        var seen = {};
        this.participants = [];
        for (var i = 0; i < this.data.length; i++) {
            var addrs = effectiveAddrs(this.data[i]);
            if (addrs.src && !seen[addrs.src]) {
                seen[addrs.src] = true;
                this.participants.push({deviceAddr: addrs.src, shortName: shortDeviceName(addrs.src)});
            }
            if (addrs.dst && !seen[addrs.dst]) {
                seen[addrs.dst] = true;
                this.participants.push({deviceAddr: addrs.dst, shortName: shortDeviceName(addrs.dst)});
            }
        }
    };

    FlowView.prototype._buildCorrelationMap = function() {
        this.correlationMap = {};
        this.correlationPairs = [];
        for (var i = 0; i < this.data.length; i++) {
            var s = this.data[i];
            if (s.msgCounter) {
                this.correlationMap[s.msgCounter] = {index: i, summary: s};
            }
        }
        for (var j = 0; j < this.data.length; j++) {
            var msg = this.data[j];
            if (msg.msgCounterRef && this.correlationMap[msg.msgCounterRef]) {
                this.correlationPairs.push({
                    reqIdx: this.correlationMap[msg.msgCounterRef].index,
                    respIdx: j
                });
            }
        }
    };

    FlowView.prototype._layoutParticipants = function() {
        var count = this.participants.length;
        if (count === 0) {
            this.laneX = [];
            this.laneWidth = LANE_MIN_WIDTH;
            return;
        }
        var w = this.container.clientWidth - LEFT_MARGIN;
        this.laneWidth = Math.max(LANE_MIN_WIDTH, Math.floor(w / count));
        this.laneX = [];
        for (var i = 0; i < count; i++) {
            this.laneX.push(LEFT_MARGIN + Math.floor(this.laneWidth * i + this.laneWidth / 2));
        }
    };

    FlowView.prototype._updateSpacer = function() {
        this.spacer.style.height = (this.data.length * ROW_HEIGHT) + 'px';
    };

    FlowView.prototype._updateCanvasSize = function() {
        var w = this.scrollEl.clientWidth;
        var h = this.scrollEl.clientHeight;
        var dpr = window.devicePixelRatio || 1;

        this.canvas.width = w * dpr;
        this.canvas.height = h * dpr;
        this.canvas.style.width = w + 'px';
        this.canvas.style.height = h + 'px';

        var ctx = this.canvas.getContext('2d');
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        // Overview canvas
        var ow = this.overviewWrap.clientWidth;
        var oh = OVERVIEW_HEIGHT;
        this.overviewCanvas.width = ow * dpr;
        this.overviewCanvas.height = oh * dpr;
        this.overviewCanvas.style.width = ow + 'px';
        this.overviewCanvas.style.height = oh + 'px';
        var octx = this.overviewCanvas.getContext('2d');
        octx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };

    FlowView.prototype._renderHeader = function() {
        this.header.innerHTML = '';
        // Left margin label
        var marginDiv = document.createElement('div');
        marginDiv.className = 'flow-participant flow-margin-label';
        marginDiv.style.width = LEFT_MARGIN + 'px';
        marginDiv.textContent = 'Time';
        this.header.appendChild(marginDiv);
        for (var i = 0; i < this.participants.length; i++) {
            var div = document.createElement('div');
            div.className = 'flow-participant';
            div.style.width = this.laneWidth + 'px';
            div.textContent = this.participants[i].shortName;
            div.title = this.participants[i].deviceAddr;
            this.header.appendChild(div);
        }
    };

    FlowView.prototype._scheduleRender = function() {
        if (this._raf) return;
        var self = this;
        this._raf = requestAnimationFrame(function() {
            self._raf = null;
            self._render();
        });
    };

    FlowView.prototype._render = function() {
        var ctx = this.canvas.getContext('2d');
        var w = this.canvas.clientWidth;
        var h = this.canvas.clientHeight;
        if (w === 0 || h === 0) return; // container hidden or not yet laid out
        ctx.clearRect(0, 0, w, h);

        if (this.data.length === 0 || this.participants.length === 0) {
            ctx.fillStyle = this._colors['_muted'];
            ctx.font = '13px ' + (getComputedStyle(document.documentElement).getPropertyValue('--font-ui').trim() || 'sans-serif');
            ctx.textAlign = 'center';
            ctx.fillText('No messages to display', w / 2, h / 2);
            return;
        }

        var scrollTop = this.scrollEl.scrollTop;
        var startIdx = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
        var endIdx = Math.min(this.data.length, Math.ceil((scrollTop + h) / ROW_HEIGHT) + OVERSCAN);

        // Build participant index for fast lookup
        var pIndex = {};
        for (var p = 0; p < this.participants.length; p++) {
            pIndex[this.participants[p].deviceAddr] = p;
        }

        // Draw lifelines (dashed for traditional UML look)
        ctx.strokeStyle = this._colors['_muted'];
        ctx.lineWidth = 1;
        ctx.setLineDash([3, 6]);
        for (var li = 0; li < this.laneX.length; li++) {
            var lx = this.laneX[li];
            ctx.beginPath();
            ctx.moveTo(lx, 0);
            ctx.lineTo(lx, h);
            ctx.stroke();
        }
        ctx.setLineDash([]);

        var fontUI = getComputedStyle(document.documentElement).getPropertyValue('--font-ui').trim() || 'sans-serif';
        var fontMono = getComputedStyle(document.documentElement).getPropertyValue('--font-mono').trim() || 'monospace';

        // Draw visible rows
        for (var i = startIdx; i < endIdx; i++) {
            var msg = this.data[i];
            var y = (i * ROW_HEIGHT) - scrollTop + ROW_HEIGHT / 2;
            var rowTop = y - ROW_HEIGHT / 2;

            // Row separator
            ctx.strokeStyle = this._colors['_border'];
            ctx.lineWidth = 1;
            ctx.setLineDash([]);
            ctx.beginPath();
            ctx.moveTo(0, rowTop + ROW_HEIGHT);
            ctx.lineTo(w, rowTop + ROW_HEIGHT);
            ctx.stroke();

            // Selected highlight + accent border
            if (i === this.selectedIndex) {
                ctx.fillStyle = this._colors['_selected'];
                ctx.fillRect(0, rowTop, w, ROW_HEIGHT);
                ctx.fillStyle = this._colors['_accent'];
                ctx.fillRect(0, rowTop, 2, ROW_HEIGHT);
            }

            // Left margin: sequence number + timestamp
            // Sequence number (right-aligned, muted)
            ctx.fillStyle = this._colors['_muted'];
            ctx.font = '11px ' + fontMono;
            ctx.textAlign = 'right';
            ctx.textBaseline = 'middle';
            if (msg.sequenceNum != null) {
                ctx.fillText(String(msg.sequenceNum), LEFT_MARGIN - 52, y);
            }
            // Timestamp HH:MM:SS.mmm
            if (msg.timestamp) {
                var ts = new Date(msg.timestamp);
                var hh = String(ts.getHours()).padStart(2, '0');
                var mm = String(ts.getMinutes()).padStart(2, '0');
                var ss = String(ts.getSeconds()).padStart(2, '0');
                var ms = String(ts.getMilliseconds()).padStart(3, '0');
                ctx.fillStyle = this._colors['_secondary'];
                ctx.fillText(hh + ':' + mm + ':' + ss + '.' + ms, LEFT_MARGIN - 6, y);
            }

            var addrs = effectiveAddrs(msg);
            var srcIdx = pIndex[addrs.src];
            var destIdx = pIndex[addrs.dst];
            var color = this._colors[msg.cmdClassifier] || this._colors['_secondary'];

            if (srcIdx === undefined && destIdx === undefined) {
                // No device info — draw a dot with label
                ctx.fillStyle = color;
                ctx.beginPath();
                ctx.arc(LEFT_MARGIN + (w - LEFT_MARGIN) / 2, y, 5, 0, Math.PI * 2);
                ctx.fill();
                if (msg.shipMsgType) {
                    ctx.fillStyle = this._colors['_secondary'];
                    ctx.font = '11px ' + fontMono;
                    ctx.textAlign = 'left';
                    ctx.textBaseline = 'middle';
                    ctx.fillText(msg.shipMsgType, LEFT_MARGIN + (w - LEFT_MARGIN) / 2 + 10, y);
                }
                continue;
            }

            var srcX, destX;

            if (srcIdx !== undefined && destIdx !== undefined && srcIdx !== destIdx) {
                srcX = this.laneX[srcIdx];
                destX = this.laneX[destIdx];
                this._drawArrow(ctx, srcX, destX, y, msg, color, fontMono);
            } else if (srcIdx !== undefined && destIdx !== undefined && srcIdx === destIdx) {
                // Self-arrow
                srcX = this.laneX[srcIdx];
                this._drawSelfArrow(ctx, srcX, y, msg, color, fontMono);
            } else if (srcIdx !== undefined) {
                srcX = this.laneX[srcIdx];
                destX = srcX + (srcIdx < this.participants.length - 1 ? 80 : -80);
                this._drawArrow(ctx, srcX, destX, y, msg, color, fontMono);
            } else {
                destX = this.laneX[destIdx];
                srcX = destX - (destIdx > 0 ? 80 : -80);
                this._drawArrow(ctx, srcX, destX, y, msg, color, fontMono);
            }
        }

        // Draw correlation pairs (dashed return lines)
        ctx.setLineDash([4, 3]);
        ctx.lineWidth = 1;
        for (var ci = 0; ci < this.correlationPairs.length; ci++) {
            var pair = this.correlationPairs[ci];
            var reqY = (pair.reqIdx * ROW_HEIGHT) - scrollTop + ROW_HEIGHT / 2;
            var respY = (pair.respIdx * ROW_HEIGHT) - scrollTop + ROW_HEIGHT / 2;

            // Skip if both are off-screen
            if ((reqY < -ROW_HEIGHT && respY < -ROW_HEIGHT) || (reqY > h + ROW_HEIGHT && respY > h + ROW_HEIGHT)) {
                continue;
            }

            var reqMsg = this.data[pair.reqIdx];
            var respMsg = this.data[pair.respIdx];
            var reqPI = pIndex[reqMsg.deviceSource];
            var respPI = pIndex[respMsg.deviceSource];

            if (reqPI === undefined || respPI === undefined) continue;

            // Draw dashed line connecting the right side of the arrow
            var offsetX = 4; // small offset from lifeline
            var reqLaneX = this.laneX[reqPI];
            var respLaneX = this.laneX[respPI];

            // Use the destination side of request (= source side of response)
            var reqDestPI = pIndex[reqMsg.deviceDest];
            if (reqDestPI === undefined) continue;
            var connX = this.laneX[reqDestPI] + offsetX;

            ctx.strokeStyle = this._colors[respMsg.cmdClassifier] || this._colors['_muted'];
            ctx.globalAlpha = 0.4;
            ctx.beginPath();
            ctx.moveTo(connX, reqY);
            ctx.lineTo(connX, respY);
            ctx.stroke();
            ctx.globalAlpha = 1.0;
        }
        ctx.setLineDash([]);
    };

    FlowView.prototype._drawArrow = function(ctx, srcX, destX, y, msg, color, fontMono) {
        var direction = destX > srcX ? 1 : -1;
        var isShipHandshake = !msg.cmdClassifier;

        // Line (dashed for SHIP handshake, solid for SPINE data)
        ctx.strokeStyle = isShipHandshake ? this._colors['_secondary'] : color;
        ctx.lineWidth = 2;
        if (isShipHandshake) {
            ctx.setLineDash([4, 3]);
        } else {
            ctx.setLineDash([]);
        }
        ctx.beginPath();
        ctx.moveTo(srcX, y);
        ctx.lineTo(destX, y);
        ctx.stroke();
        ctx.setLineDash([]);

        // Arrowhead
        ctx.fillStyle = isShipHandshake ? this._colors['_secondary'] : color;
        ctx.beginPath();
        ctx.moveTo(destX, y);
        ctx.lineTo(destX - direction * ARROW_HEAD, y - ARROW_HEAD / 2);
        ctx.lineTo(destX - direction * ARROW_HEAD, y + ARROW_HEAD / 2);
        ctx.closePath();
        ctx.fill();

        // Label
        var label = msg.functionSet || msg.shipMsgType || '';
        if (label) {
            var midX = (srcX + destX) / 2;
            ctx.fillStyle = isShipHandshake ? this._colors['_secondary'] : color;
            ctx.font = '11px ' + fontMono;
            ctx.textAlign = 'center';
            ctx.textBaseline = 'bottom';

            // Truncate label if too long
            var maxLabelWidth = Math.abs(destX - srcX) - 16;
            if (maxLabelWidth > 20) {
                var text = label;
                while (ctx.measureText(text).width > maxLabelWidth && text.length > 4) {
                    text = text.slice(0, -2) + '\u2026';
                }
                ctx.fillText(text, midX, y - 3);
            }
        }
    };

    FlowView.prototype._drawSelfArrow = function(ctx, x, y, msg, color, fontMono) {
        var isShipHandshake = !msg.cmdClassifier;
        ctx.strokeStyle = isShipHandshake ? this._colors['_secondary'] : color;
        ctx.lineWidth = 2;
        if (isShipHandshake) {
            ctx.setLineDash([4, 3]);
        } else {
            ctx.setLineDash([]);
        }
        ctx.beginPath();
        ctx.moveTo(x, y);
        ctx.lineTo(x + SELF_ARROW_WIDTH, y);
        ctx.lineTo(x + SELF_ARROW_WIDTH, y + ROW_HEIGHT * 0.6);
        ctx.lineTo(x, y + ROW_HEIGHT * 0.6);
        ctx.stroke();
        ctx.setLineDash([]);

        // Arrowhead
        ctx.fillStyle = isShipHandshake ? this._colors['_secondary'] : color;
        ctx.beginPath();
        ctx.moveTo(x, y + ROW_HEIGHT * 0.6);
        ctx.lineTo(x + ARROW_HEAD, y + ROW_HEIGHT * 0.6 - ARROW_HEAD / 2);
        ctx.lineTo(x + ARROW_HEAD, y + ROW_HEIGHT * 0.6 + ARROW_HEAD / 2);
        ctx.closePath();
        ctx.fill();

        // Label
        var label = msg.functionSet || msg.shipMsgType || '';
        if (label) {
            ctx.fillStyle = isShipHandshake ? this._colors['_secondary'] : color;
            ctx.font = '11px ' + fontMono;
            ctx.textAlign = 'left';
            ctx.textBaseline = 'bottom';
            ctx.fillText(label, x + SELF_ARROW_WIDTH + 4, y + 2);
        }
    };

    FlowView.prototype._renderOverview = function() {
        var ctx = this.overviewCanvas.getContext('2d');
        var w = this.overviewCanvas.clientWidth;
        var h = OVERVIEW_HEIGHT;
        ctx.clearRect(0, 0, w, h);

        if (this.data.length === 0 || this.participants.length === 0 || w === 0) return;

        var total = this.data.length;
        var bucketCount = Math.min(w, total);
        if (bucketCount === 0) return;
        var bucketSize = Math.max(1, Math.floor(total / bucketCount));
        var pCount = this.participants.length;
        var laneW = w / pCount;

        // Build participant index
        var pIndex = {};
        for (var p = 0; p < this.participants.length; p++) {
            pIndex[this.participants[p].deviceAddr] = p;
        }

        // Compute density per bucket per participant
        var density = [];
        for (var b = 0; b < bucketCount; b++) {
            density.push(new Array(pCount).fill(0));
        }
        var maxDensity = 1;
        for (var i = 0; i < total; i++) {
            var bi = Math.min(Math.floor(i / bucketSize), bucketCount - 1);
            var msg = this.data[i];
            var si = pIndex[msg.deviceSource];
            var di = pIndex[msg.deviceDest];
            if (si !== undefined) {
                density[bi][si]++;
                if (density[bi][si] > maxDensity) maxDensity = density[bi][si];
            }
            if (di !== undefined) {
                density[bi][di]++;
                if (density[bi][di] > maxDensity) maxDensity = density[bi][di];
            }
        }

        // Draw density blocks
        var colW = Math.max(1, w / bucketCount);
        for (var bj = 0; bj < bucketCount; bj++) {
            for (var pj = 0; pj < pCount; pj++) {
                var val = density[bj][pj];
                if (val === 0) continue;
                var alpha = Math.min(0.8, 0.1 + (val / maxDensity) * 0.7);
                ctx.fillStyle = this._colors[Object.keys(CLASSIFIER_COLORS)[pj % Object.keys(CLASSIFIER_COLORS).length]] || this._colors['_accent'];
                ctx.globalAlpha = alpha;
                var rx = bj * colW;
                var ry = (pj / pCount) * h;
                var rh = h / pCount;
                ctx.fillRect(rx, ry, colW, rh);
            }
        }
        ctx.globalAlpha = 1.0;

        // Participant labels on overview
        ctx.fillStyle = this._colors['_secondary'];
        ctx.font = '9px ' + (getComputedStyle(document.documentElement).getPropertyValue('--font-ui').trim() || 'sans-serif');
        ctx.textAlign = 'left';
        for (var pl = 0; pl < this.participants.length; pl++) {
            var ly = ((pl + 0.5) / pCount) * h;
            ctx.fillText(this.participants[pl].shortName, 4, ly + 3);
        }

        // Viewport indicator
        var viewH = this.scrollEl.clientHeight;
        var totalH = this.data.length * ROW_HEIGHT;
        if (totalH > 0) {
            var vpStart = this.scrollEl.scrollTop / totalH;
            var vpEnd = (this.scrollEl.scrollTop + viewH) / totalH;
            var vpX1 = vpStart * w;
            var vpX2 = vpEnd * w;

            ctx.strokeStyle = this._colors['_accent'];
            ctx.lineWidth = 1.5;
            ctx.strokeRect(vpX1, 0, vpX2 - vpX1, h);
            ctx.fillStyle = this._colors['_accent'];
            ctx.globalAlpha = 0.06;
            ctx.fillRect(vpX1, 0, vpX2 - vpX1, h);
            ctx.globalAlpha = 1.0;
        }

        // Selected indicator
        if (this.selectedIndex >= 0 && this.selectedIndex < total) {
            var selRatio = this.selectedIndex / total;
            var selX = selRatio * w;
            ctx.strokeStyle = this._colors['_accent'];
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(selX, 0);
            ctx.lineTo(selX, h);
            ctx.stroke();
        }
    };

    // --- Helpers ---

    function shortDeviceName(addr) {
        if (!addr) return '';
        // EEBus device address: split on last underscore
        for (var i = addr.length - 1; i >= 0; i--) {
            if (addr[i] === '_') return addr.substring(i + 1);
        }
        // IP:port — strip port for display
        var bracketIdx = addr.lastIndexOf(']');
        if (bracketIdx >= 0) {
            var colonAfter = addr.indexOf(':', bracketIdx);
            if (colonAfter >= 0) return addr.substring(0, colonAfter);
        } else {
            var lastColon = addr.lastIndexOf(':');
            if (lastColon > 0 && /^\d/.test(addr.substring(lastColon + 1))) {
                return addr.substring(0, lastColon);
            }
        }
        return addr;
    }

    // Return effective source/dest addresses for a summary, falling back
    // to network addresses when SPINE device addresses are not available.
    function effectiveAddrs(s) {
        var src = s.deviceSource, dst = s.deviceDest;
        if (!src && !dst) { src = s.sourceAddr; dst = s.destAddr; }
        return {src: src || '', dst: dst || ''};
    }

    window.FlowView = FlowView;
})();

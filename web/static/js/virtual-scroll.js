// VirtualScroll — renders only visible rows from a large data array.
//
// Usage:
//   var vs = new VirtualScroll({
//       container: document.querySelector('.message-table-container'),
//       tbody:     document.querySelector('#message-tbody'),
//       rowHeight: 24,
//       overscan:  10,
//       renderRow: function(item, index) { return '<tr>...</tr>'; },
//       onSelect:  function(item, index) { ... }
//   });
//   vs.setData(items);

(function() {
    'use strict';

    function VirtualScroll(opts) {
        this.container = opts.container;
        this.tbody     = opts.tbody;
        this.rowHeight = opts.rowHeight || 24;
        this.overscan  = opts.overscan  || 10;
        this.renderRow = opts.renderRow;
        this.onSelect  = opts.onSelect  || null;

        this.data = [];
        this.selectedIndex = -1;
        this._startIndex = 0;
        this._endIndex = 0;
        this._rafPending = false;

        // Create spacer rows
        this._spacerTop = document.createElement('tr');
        this._spacerTop.className = 'vs-spacer';
        this._spacerBottom = document.createElement('tr');
        this._spacerBottom.className = 'vs-spacer';

        this._onScroll = this._handleScroll.bind(this);
        this.container.addEventListener('scroll', this._onScroll);
    }

    VirtualScroll.prototype.setData = function(items) {
        this.data = items || [];
        this.selectedIndex = -1;
        this.container.scrollTop = 0;
        this._render();
    };

    VirtualScroll.prototype.getItem = function(index) {
        return this.data[index] || null;
    };

    VirtualScroll.prototype.getSelectedIndex = function() {
        return this.selectedIndex;
    };

    VirtualScroll.prototype.setSelectedIndex = function(index) {
        this.selectedIndex = index;
        this._updateSelectedClass();
    };

    VirtualScroll.prototype.scrollToIndex = function(index) {
        if (index < 0 || index >= this.data.length) return;
        var targetTop = index * this.rowHeight;
        var viewH = this.container.clientHeight;
        var scrollTop = this.container.scrollTop;

        // If already visible, don't scroll
        if (targetTop >= scrollTop && targetTop + this.rowHeight <= scrollTop + viewH) {
            return;
        }

        // Scroll to center the target row
        this.container.scrollTop = targetTop - viewH / 2 + this.rowHeight / 2;
    };

    VirtualScroll.prototype.refresh = function() {
        this._render();
    };

    VirtualScroll.prototype.appendItem = function(item) {
        this.data.push(item);
        // If we're near the bottom, re-render to include the new item
        var totalH = this.data.length * this.rowHeight;
        var scrollBottom = this.container.scrollTop + this.container.clientHeight;
        if (totalH - scrollBottom < this.rowHeight * (this.overscan + 5)) {
            this._render();
        } else {
            // Just update bottom spacer height
            this._updateSpacers(this._startIndex, this._endIndex);
        }
    };

    VirtualScroll.prototype.getVisibleRange = function() {
        return { start: this._startIndex, end: this._endIndex };
    };

    VirtualScroll.prototype.destroy = function() {
        this.container.removeEventListener('scroll', this._onScroll);
        this.data = [];
        this.tbody.innerHTML = '';
    };

    // --- Internal ---

    VirtualScroll.prototype._handleScroll = function() {
        if (this._rafPending) return;
        this._rafPending = true;
        var self = this;
        requestAnimationFrame(function() {
            self._rafPending = false;
            self._render();
        });
    };

    VirtualScroll.prototype._calcRange = function() {
        var scrollTop = this.container.scrollTop;
        var viewH = this.container.clientHeight;
        var totalItems = this.data.length;

        if (totalItems === 0) return { start: 0, end: 0 };

        var startIndex = Math.floor(scrollTop / this.rowHeight) - this.overscan;
        if (startIndex < 0) startIndex = 0;

        var visibleCount = Math.ceil(viewH / this.rowHeight);
        var endIndex = startIndex + visibleCount + 2 * this.overscan;
        if (endIndex > totalItems) endIndex = totalItems;

        return { start: startIndex, end: endIndex };
    };

    VirtualScroll.prototype._render = function() {
        var range = this._calcRange();
        var start = range.start;
        var end = range.end;

        // Skip re-render if range hasn't changed
        if (start === this._startIndex && end === this._endIndex && this.tbody.childNodes.length > 2) {
            return;
        }

        this._startIndex = start;
        this._endIndex = end;

        // Build HTML for visible rows
        var html = '';
        for (var i = start; i < end; i++) {
            html += this.renderRow(this.data[i], i);
        }

        // Replace tbody content: spacerTop + rows + spacerBottom
        this.tbody.innerHTML = '';
        this._updateSpacers(start, end);
        this.tbody.appendChild(this._spacerTop);

        // Insert rows via a temporary container for performance
        var temp = document.createElement('tbody');
        temp.innerHTML = html;
        while (temp.firstChild) {
            this.tbody.appendChild(temp.firstChild);
        }

        this.tbody.appendChild(this._spacerBottom);

        // Re-apply selected class
        this._updateSelectedClass();

        // Wire up click handlers for onSelect
        if (this.onSelect) {
            this._wireClickHandlers(start);
        }
    };

    VirtualScroll.prototype._updateSpacers = function(start, end) {
        var totalItems = this.data.length;
        var topH = start * this.rowHeight;
        var bottomH = (totalItems - end) * this.rowHeight;
        if (bottomH < 0) bottomH = 0;

        // Use a single cell spanning all columns
        this._spacerTop.innerHTML = '<td colspan="99" style="height:' + topH + 'px;padding:0;border:0"></td>';
        this._spacerBottom.innerHTML = '<td colspan="99" style="height:' + bottomH + 'px;padding:0;border:0"></td>';
    };

    VirtualScroll.prototype._updateSelectedClass = function() {
        var rows = this.tbody.querySelectorAll('.msg-row');
        for (var i = 0; i < rows.length; i++) {
            rows[i].classList.remove('selected');
        }
        if (this.selectedIndex >= this._startIndex && this.selectedIndex < this._endIndex) {
            var offset = this.selectedIndex - this._startIndex;
            // +1 to skip spacer top row
            var row = this.tbody.children[offset + 1];
            if (row && row.classList.contains('msg-row')) {
                row.classList.add('selected');
            }
        }
    };

    VirtualScroll.prototype._wireClickHandlers = function(startIndex) {
        var self = this;
        var rows = this.tbody.querySelectorAll('.msg-row');
        for (var i = 0; i < rows.length; i++) {
            (function(idx) {
                rows[idx].addEventListener('click', function() {
                    var dataIdx = startIndex + idx;
                    self.selectedIndex = dataIdx;
                    self._updateSelectedClass();
                    if (self.onSelect) {
                        self.onSelect(self.data[dataIdx], dataIdx);
                    }
                });
            })(i);
        }
    };

    // Expose globally
    window.VirtualScroll = VirtualScroll;
})();

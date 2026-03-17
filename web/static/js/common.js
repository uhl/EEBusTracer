// EEBusTracer shared utilities
// Used by all visualization pages and app.js

(function() {
    'use strict';

    window.EEBusTracer = window.EEBusTracer || {};

    // Read a CSS custom property from the document root
    function cssVar(name) {
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    }
    EEBusTracer.cssVar = cssVar;

    // Command classifier color palette — reads from CSS variables
    function refreshCmdColors() {
        EEBusTracer.CMD_COLORS = {
            read:   cssVar('--badge-read-fg'),
            reply:  cssVar('--badge-reply-fg'),
            notify: cssVar('--badge-notify-fg'),
            write:  cssVar('--badge-write-fg'),
            call:   cssVar('--badge-call-fg'),
            result: cssVar('--badge-result-fg')
        };
    }
    refreshCmdColors();

    // Re-read colors on theme change
    document.addEventListener('theme-changed', refreshCmdColors);

    // HTML escaping
    EEBusTracer.escapeHtml = function(text) {
        if (!text) return '';
        return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    };

    // JSON syntax highlighting — reads colors from CSS variables each call
    EEBusTracer.syntaxHighlight = function(json) {
        if (!json) return '';
        json = json.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        return json.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
            function(match) {
                var cls = 'color: ' + cssVar('--syntax-number') + ';'; // number
                if (/^"/.test(match)) {
                    if (/:$/.test(match)) {
                        cls = 'color: ' + cssVar('--syntax-key') + ';'; // key
                    } else {
                        cls = 'color: ' + cssVar('--syntax-string') + ';'; // string
                    }
                } else if (/true|false/.test(match)) {
                    cls = 'color: ' + cssVar('--syntax-boolean') + ';'; // boolean
                } else if (/null/.test(match)) {
                    cls = 'color: ' + cssVar('--syntax-null') + ';'; // null
                }
                return '<span style="' + cls + '">' + match + '</span>';
            }
        );
    };

    // Hex dump formatting
    EEBusTracer.formatHex = function(hex) {
        if (!hex) return '(empty)';
        var result = '';
        for (var i = 0; i < hex.length; i += 2) {
            if (i > 0 && i % 32 === 0) result += '\n';
            else if (i > 0) result += ' ';
            result += hex.substring(i, i + 2);
        }
        return result;
    };

    // API fetch helper with error handling
    EEBusTracer.apiFetch = function(url, options) {
        return fetch(url, options).then(function(resp) {
            if (!resp.ok) {
                return resp.json().then(function(err) {
                    throw new Error(err.error || 'Request failed');
                });
            }
            return resp.json();
        });
    };

    // Format timestamp for display
    EEBusTracer.formatTimestamp = function(ts) {
        var d = new Date(ts);
        return d.toTimeString().substring(0, 8) + '.' + String(d.getMilliseconds()).padStart(3, '0');
    };

    // Get color for a command classifier
    EEBusTracer.cmdColor = function(classifier) {
        return EEBusTracer.CMD_COLORS[classifier] || cssVar('--text-muted');
    };

})();

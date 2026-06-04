var HeatmapLayer = (function() {
    'use strict';

    var Heatmap = function(options) {
        this.map = options.map;
        this.apiBase = options.apiBase || window.location.origin;
        this.baselineTemp = 0;
        this.heatmapData = null;
        this.heatmapOverlay = null;
        this.stationMarkers = {};
        this.onStationClick = options.onStationClick || function() {};

        this.offscreenCanvas = document.createElement('canvas');
        this.offscreenCanvas.width = 2048;
        this.offscreenCanvas.height = 2048;
        this.offscreenCtx = this.offscreenCanvas.getContext('2d');

        this.displayCanvas = document.createElement('canvas');
        this.displayCtx = this.displayCanvas.getContext('2d');

        this.cacheMinLng = 0;
        this.cacheMaxLng = 0;
        this.cacheMinLat = 0;
        this.cacheMaxLat = 0;
        this.lastDataHash = 0;
        this.heatmapDirty = true;
        this.renderFrameId = null;

        this._init();
    };

    Heatmap.prototype._init = function() {
        this.map.on('moveend', this._scheduleRender.bind(this));
        this.map.on('zoomend', this._scheduleRender.bind(this));
        this.map.on('resize', this._onResize.bind(this));
        this._onResize();
    };

    Heatmap.prototype._onResize = function() {
        var size = this.map.getSize();
        this.displayCanvas.width = size.x;
        this.displayCanvas.height = size.y;
        this._scheduleRender();
    };

    Heatmap.prototype.loadData = function(callback) {
        var self = this;
        fetch(this.apiBase + '/api/heatmap')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                self.heatmapData = data;
                self._updateBaselineTemp(data);
                self._updateStationMarkers(data);
                self.heatmapDirty = true;
                self._scheduleRender();
                if (callback) callback(null, data);
            })
            .catch(function(err) {
                console.error('Heatmap load error:', err);
                if (callback) callback(err);
            });
    };

    Heatmap.prototype._updateBaselineTemp = function(data) {
        var baselinePoints = data.filter(function(p) {
            return p.is_baseline && typeof p.temperature === 'number' && !isNaN(p.temperature);
        });

        if (baselinePoints.length >= 2) {
            var sum = baselinePoints.reduce(function(s, p) { return s + p.temperature; }, 0);
            var avg = sum / baselinePoints.length;
            if (avg > -40 && avg < 60) {
                this.baselineTemp = avg;
            }
        }
    };

    Heatmap.prototype._hashHeatmapData = function(data) {
        if (!data) return 0;
        var hash = 0;
        for (var i = 0; i < data.length; i++) {
            var p = data[i];
            hash = (hash * 31 + Math.round(p.temperature * 100)) | 0;
            hash = (hash * 31 + Math.round(p.latitude * 10000)) | 0;
            hash = (hash * 31 + Math.round(p.longitude * 10000)) | 0;
        }
        return hash;
    };

    Heatmap.prototype._scheduleRender = function() {
        if (this.renderFrameId) {
            cancelAnimationFrame(this.renderFrameId);
        }
        this.renderFrameId = requestAnimationFrame(this._render.bind(this));
    };

    Heatmap.prototype._render = function() {
        if (!this.heatmapData) return;

        var size = this.map.getSize();
        if (this.displayCanvas.width !== size.x || this.displayCanvas.height !== size.y) {
            this.displayCanvas.width = size.x;
            this.displayCanvas.height = size.y;
        }
        this.displayCtx.clearRect(0, 0, size.x, size.y);

        var points = this.heatmapData.filter(function(p) { return !p.is_baseline; });
        if (points.length === 0) return;

        var currentHash = this._hashHeatmapData(points);
        if (currentHash !== this.lastDataHash) {
            this.heatmapDirty = true;
            this.lastDataHash = currentHash;
        }

        if (this.heatmapDirty) {
            this._rebuildOffscreenCache(points);
            this.heatmapDirty = false;
        }

        var bounds = this.map.getBounds();
        var sw = bounds.getSouthWest();
        var ne = bounds.getNorthEast();

        var lngRange = this.cacheMaxLng - this.cacheMinLng;
        var latRange = this.cacheMaxLat - this.cacheMinLat;

        var srcX = (sw.lng - this.cacheMinLng) / lngRange * this.offscreenCanvas.width;
        var srcY = (this.cacheMaxLat - ne.lat) / latRange * this.offscreenCanvas.height;
        var srcW = (ne.lng - sw.lng) / lngRange * this.offscreenCanvas.width;
        var srcH = (ne.lat - sw.lat) / latRange * this.offscreenCanvas.height;

        this.displayCtx.drawImage(
            this.offscreenCanvas,
            Math.max(0, srcX), Math.max(0, srcY),
            Math.max(1, srcW), Math.max(1, srcH),
            0, 0, size.x, size.y
        );

        if (this.heatmapOverlay) {
            this.heatmapOverlay.remove();
        }
        var imgDataUrl = this.displayCanvas.toDataURL();
        this.heatmapOverlay = L.imageOverlay(imgDataUrl, bounds, {
            opacity: 0.55,
            interactive: false
        }).addTo(this.map);
    };

    Heatmap.prototype._rebuildOffscreenCache = function(points) {
        this.offscreenCtx.clearRect(0, 0, this.offscreenCanvas.width, this.offscreenCanvas.height);

        this.cacheMinLng = Infinity; this.cacheMaxLng = -Infinity;
        this.cacheMinLat = Infinity; this.cacheMaxLat = -Infinity;
        points.forEach(function(p) {
            if (p.longitude < this.cacheMinLng) this.cacheMinLng = p.longitude;
            if (p.longitude > this.cacheMaxLng) this.cacheMaxLng = p.longitude;
            if (p.latitude < this.cacheMinLat) this.cacheMinLat = p.latitude;
            if (p.latitude > this.cacheMaxLat) this.cacheMaxLat = p.latitude;
        }.bind(this));
        this.cacheMinLng -= 0.02; this.cacheMaxLng += 0.02;
        this.cacheMinLat -= 0.02; this.cacheMaxLat += 0.02;

        var maxTemp = -Infinity, minTemp = Infinity;
        points.forEach(function(p) {
            if (p.temperature > maxTemp) maxTemp = p.temperature;
            if (p.temperature < minTemp) minTemp = p.temperature;
        });
        var range = maxTemp - minTemp || 1;

        var lngRange = this.cacheMaxLng - this.cacheMinLng;
        var latRange = this.cacheMaxLat - this.cacheMinLat;

        points.forEach(function(p) {
            var x = (p.longitude - this.cacheMinLng) / lngRange * this.offscreenCanvas.width;
            var y = (this.cacheMaxLat - p.latitude) / latRange * this.offscreenCanvas.height;
            var norm = (p.temperature - minTemp) / range;
            var baseRadius = 0.03 / lngRange * this.offscreenCanvas.width;
            var radius = baseRadius + norm * baseRadius * 0.7;

            var r, g, b;
            if (norm < 0.5) {
                r = Math.round(norm * 2 * 255);
                g = 200;
                b = Math.round((1 - norm * 2) * 100);
            } else {
                r = 255;
                g = Math.round((1 - (norm - 0.5) * 2) * 200);
                b = 0;
            }

            var gradient = this.offscreenCtx.createRadialGradient(x, y, 0, x, y, radius);
            gradient.addColorStop(0, 'rgba(' + r + ',' + g + ',' + b + ',0.5)');
            gradient.addColorStop(0.4, 'rgba(' + r + ',' + g + ',' + b + ',0.25)');
            gradient.addColorStop(1, 'rgba(' + r + ',' + g + ',' + b + ',0)');

            this.offscreenCtx.fillStyle = gradient;
            this.offscreenCtx.beginPath();
            this.offscreenCtx.arc(x, y, radius, 0, Math.PI * 2);
            this.offscreenCtx.fill();
        }.bind(this));
    };

    Heatmap.prototype._updateStationMarkers = function(data) {
        var weatherMap = {};
        data.forEach(function(p) { weatherMap[p.station_id] = p; });

        for (var i = 0; i < data.length; i++) {
            var station = data[i];
            this._createOrUpdateMarker(station, weatherMap[station.station_id]);
        }
    };

    Heatmap.prototype._createOrUpdateMarker = function(station, weatherData) {
        var heatIndex = weatherData ? (weatherData.temperature - this.baselineTemp) : 0;
        var color = station.is_baseline ? '#42a5f5' : this._getStationColor(heatIndex);
        var radius = station.is_baseline ? 8 : this._getStationRadius(heatIndex);

        var icon = L.divIcon({
            className: '',
            html: '<div style="' +
                'width:' + (radius*2) + 'px;' +
                'height:' + (radius*2) + 'px;' +
                'border-radius:50%;' +
                'background:' + color + ';' +
                'border:2px solid rgba(255,255,255,0.6);' +
                'box-shadow:0 0 ' + (radius) + 'px ' + color + '80;' +
                'cursor:pointer;' +
                'transition:all 0.3s;' +
            '"></div>',
            iconSize: [radius*2, radius*2],
            iconAnchor: [radius, radius]
        });

        if (this.stationMarkers[station.station_id]) {
            this.stationMarkers[station.station_id].setIcon(icon);
            this.stationMarkers[station.station_id].setLatLng([station.latitude, station.longitude]);
            return;
        }

        var marker = L.marker([station.latitude, station.longitude], { icon: icon }).addTo(this.map);
        marker.stationData = station;
        marker.on('click', function() {
            this.onStationClick(station);
        }.bind(this));
        this.stationMarkers[station.station_id] = marker;
    };

    Heatmap.prototype._getStationColor = function(heatIndex) {
        if (heatIndex < 2) return '#4caf50';
        if (heatIndex <= 5) return '#ffc107';
        return '#f44336';
    };

    Heatmap.prototype._getStationRadius = function(heatIndex) {
        return Math.max(8, Math.min(20, 8 + heatIndex * 2));
    };

    Heatmap.prototype.getBaselineTemp = function() {
        return this.baselineTemp;
    };

    Heatmap.prototype.destroy = function() {
        if (this.renderFrameId) {
            cancelAnimationFrame(this.renderFrameId);
        }
        if (this.heatmapOverlay) {
            this.heatmapOverlay.remove();
        }
        for (var id in this.stationMarkers) {
            this.map.removeLayer(this.stationMarkers[id]);
        }
        this.stationMarkers = {};
    };

    return Heatmap;
})();

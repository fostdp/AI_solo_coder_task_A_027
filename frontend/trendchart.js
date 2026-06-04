var TrendChart = (function() {
    'use strict';

    var Chart = window.Chart;

    var TrendChart = function(options) {
        this.canvasId = options.canvasId;
        this.apiBase = options.apiBase || window.location.origin;
        this.title = options.title || '热岛强度趋势';
        this.days = options.days || 7;
        this.chart = null;
        this._init();
    };

    TrendChart.prototype._init = function() {
        var ctx = document.getElementById(this.canvasId);
        if (!ctx) {
            console.error('TrendChart: Canvas not found:', this.canvasId);
            return;
        }

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: [],
                datasets: [{
                    label: '热岛强度(℃)',
                    data: [],
                    borderColor: '#ff9800',
                    backgroundColor: 'rgba(255,152,0,0.15)',
                    borderWidth: 2,
                    pointRadius: 0,
                    fill: true,
                    tension: 0.3
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        labels: {
                            color: '#7b8ea0',
                            font: { size: 11 }
                        }
                    },
                    tooltip: {
                        mode: 'index',
                        intersect: false,
                        backgroundColor: 'rgba(13,27,42,0.95)',
                        titleColor: '#4fc3f7',
                        bodyColor: '#e0e6ed',
                        borderColor: '#1e3a5f',
                        borderWidth: 1
                    }
                },
                scales: {
                    x: {
                        ticks: {
                            color: '#7b8ea0',
                            font: { size: 10 },
                            maxTicksLimit: 10
                        },
                        grid: { color: '#1a2d42' }
                    },
                    y: {
                        ticks: {
                            color: '#7b8ea0',
                            font: { size: 10 }
                        },
                        grid: { color: '#1a2d42' },
                        suggestedMin: 0
                    }
                },
                interaction: {
                    mode: 'nearest',
                    axis: 'x',
                    intersect: false
                }
            }
        });
    };

    TrendChart.prototype.load = function(days, callback) {
        if (typeof days === 'function') {
            callback = days;
            days = this.days;
        }
        if (days) {
            this.days = days;
        }

        var self = this;
        fetch(this.apiBase + '/api/heat-island/trend?days=' + this.days)
            .then(function(r) { return r.json(); })
            .then(function(data) {
                self._updateChart(data);
                if (callback) callback(null, data);
            })
            .catch(function(err) {
                console.error('TrendChart load error:', err);
                if (callback) callback(err);
            });
    };

    TrendChart.prototype._updateChart = function(data) {
        if (!this.chart || !data || data.length === 0) return;

        var labels = data.map(function(d) {
            var t = new Date(d.time);
            return (t.getMonth()+1) + '/' + t.getDate() + ' ' + t.getHours() + ':00';
        });
        var intensities = data.map(function(d) { return d.intensity; });

        this.chart.data.labels = labels;
        this.chart.data.datasets[0].data = intensities;
        this.chart.update('none');
    };

    TrendChart.prototype.getCurrent = function(callback) {
        var self = this;
        fetch(this.apiBase + '/api/heat-island/current')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (callback) callback(null, data);
            })
            .catch(function(err) {
                console.error('GetCurrent error:', err);
                if (callback) callback(err);
            });
    };

    TrendChart.prototype.update = function() {
        this.load();
    };

    TrendChart.prototype.destroy = function() {
        if (this.chart) {
            this.chart.destroy();
            this.chart = null;
        }
    };

    return TrendChart;
})();

var StationChart = (function() {
    'use strict';

    var Chart = window.Chart;

    var StationChart = function(options) {
        this.canvasId = options.canvasId;
        this.apiBase = options.apiBase || window.location.origin;
        this.stationId = options.stationId;
        this.chart = null;
        this._init();
    };

    StationChart.prototype._init = function() {
        var ctx = document.getElementById(this.canvasId);
        if (!ctx) return;

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: [],
                datasets: [
                    {
                        label: '温度(℃)',
                        data: [],
                        borderColor: '#ef5350',
                        backgroundColor: 'rgba(239,83,80,0.1)',
                        borderWidth: 1.5,
                        pointRadius: 0,
                        fill: true,
                        tension: 0.3,
                        yAxisID: 'y'
                    },
                    {
                        label: '湿度(%)',
                        data: [],
                        borderColor: '#42a5f5',
                        backgroundColor: 'rgba(66,165,245,0.1)',
                        borderWidth: 1.5,
                        pointRadius: 0,
                        fill: true,
                        tension: 0.3,
                        yAxisID: 'y1'
                    }
                ]
            },
            options: {
                responsive: false,
                plugins: {
                    legend: {
                        display: true,
                        labels: {
                            color: '#7b8ea0',
                            font: { size: 10 }
                        }
                    }
                },
                scales: {
                    x: {
                        ticks: {
                            color: '#7b8ea0',
                            font: { size: 9 },
                            maxTicksLimit: 8
                        },
                        grid: { color: '#1a2d42' }
                    },
                    y: {
                        position: 'left',
                        ticks: {
                            color: '#ef5350',
                            font: { size: 9 }
                        },
                        grid: { color: '#1a2d42' }
                    },
                    y1: {
                        position: 'right',
                        ticks: {
                            color: '#42a5f5',
                            font: { size: 9 }
                        },
                        grid: { display: false }
                    }
                }
            }
        });
    };

    StationChart.prototype.load24h = function(stationId, callback) {
        if (stationId) {
            this.stationId = stationId;
        }
        if (!this.stationId) {
            if (callback) callback(new Error('No station ID'));
            return;
        }

        var self = this;
        fetch(this.apiBase + '/api/stations/' + this.stationId + '/24h')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                self._updateChart(data);
                if (callback) callback(null, data);
            })
            .catch(function(err) {
                console.error('StationChart load error:', err);
                if (callback) callback(err);
            });
    };

    StationChart.prototype._updateChart = function(data) {
        if (!this.chart || !data || data.length === 0) return;

        var labels = data.map(function(d) {
            var t = new Date(d.time);
            return t.getHours() + ':' + String(t.getMinutes()).padStart(2, '0');
        });
        var temps = data.map(function(d) { return d.temperature; });
        var hums = data.map(function(d) { return d.humidity; });

        this.chart.data.labels = labels;
        this.chart.data.datasets[0].data = temps;
        this.chart.data.datasets[1].data = hums;
        this.chart.update('none');
    };

    StationChart.prototype.destroy = function() {
        if (this.chart) {
            this.chart.destroy();
            this.chart = null;
        }
    };

    return StationChart;
})();

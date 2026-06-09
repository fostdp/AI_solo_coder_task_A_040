const App = (function() {
    let detectors = [];
    let leakSources = [];
    let refreshInterval = null;

    function init() {
        console.log('初始化应用...');
        
        CorridorMapModule.init();
        GasPanelModule.init();
        HeatmapModule.init();
        
        setupEventListeners();
        setupWebSocketCallbacks();
        
        loadInitialData();
        
        refreshInterval = setInterval(refreshData, 5000);
        WebSocketModule.connect();
    }

    function setupEventListeners() {
        document.getElementById('btn-refresh').addEventListener('click', refreshData);
        
        document.getElementById('btn-toggle-detectors').addEventListener('click', function() {
            const visible = CorridorMapModule.toggleDetectors();
            this.textContent = visible ? '隐藏检测器' : '显示检测器';
        });

        document.getElementById('btn-toggle-heatmap').addEventListener('click', function() {
            const visible = HeatmapModule.toggle();
            this.textContent = visible ? '隐藏热力图' : '显示热力图';
        });

        document.getElementById('btn-toggle-valves').addEventListener('click', function() {
            const visible = CorridorMapModule.toggleValves();
            this.textContent = visible ? '隐藏阀门' : '显示阀门';
        });

        document.getElementById('btn-toggle-leaks').addEventListener('click', function() {
            const visible = CorridorMapModule.toggleLeaks();
            this.textContent = visible ? '隐藏泄漏源' : '显示泄漏源';
        });

        document.getElementById('btn-acknowledge-alarm').addEventListener('click', function() {
            GasPanelModule.acknowledgeSelectedAlarm();
        });

        document.getElementById('btn-close-modal').addEventListener('click', function() {
            GasPanelModule.closeDetectorModal();
        });
        
        document.querySelector('.modal-overlay').addEventListener('click', function(e) {
            if (e.target === this) {
                GasPanelModule.closeDetectorModal();
            }
        });

        document.getElementById('alarm-list').addEventListener('click', function(e) {
            const alarmItem = e.target.closest('.alarm-item');
            if (alarmItem) {
                const alarmId = alarmItem.dataset.alarmId;
                GasPanelModule.focusOnAlarm(alarmId);
            }
        });
    }

    function setupWebSocketCallbacks() {
        WebSocketModule.setOnConcentrationUpdate(function(data) {
            updateDetectorConcentrations(data);
        });

        WebSocketModule.setOnAlarmUpdate(function(data) {
            GasPanelModule.handleAlarmUpdate(data);
        });

        WebSocketModule.setOnLeakSourceUpdate(function(data) {
            handleLeakSourceUpdate(data);
        });

        WebSocketModule.setOnStatusUpdate(function(data) {
            console.log('WebSocket状态:', data);
        });
    }

    async function loadInitialData() {
        try {
            await Promise.all([
                loadDetectors(),
                GasPanelModule.loadActiveAlarms(),
                loadLeakSources(),
                GasPanelModule.loadStatistics()
            ]);
            
            CorridorMapModule.loadPipeCorridor();
            CorridorMapModule.addDetectorMarkers(detectors);
            CorridorMapModule.addValveMarkers();
            
            HeatmapModule.updateHeatmap(detectors);
            
        } catch (error) {
            console.error('加载初始数据失败:', error);
            GasPanelModule.showNotification('加载数据失败，请刷新页面重试', 'error');
        }
    }

    async function loadDetectors() {
        try {
            const response = await fetch(`${Config.API_URL}/detectors`);
            if (!response.ok) throw new Error('获取检测器列表失败');
            detectors = await response.json();
            CorridorMapModule.setDetectors(detectors);
            GasPanelModule.setDetectors(detectors);
            return detectors;
        } catch (error) {
            console.error('加载检测器失败:', error);
            throw error;
        }
    }

    async function loadLeakSources() {
        try {
            const response = await fetch(`${Config.API_URL}/leaks/active`);
            if (!response.ok) throw new Error('获取泄漏源失败');
            leakSources = await response.json();
            CorridorMapModule.updateLeakMarkers(leakSources);
            return leakSources;
        } catch (error) {
            console.error('加载泄漏源失败:', error);
            throw error;
        }
    }

    async function refreshData() {
        try {
            await Promise.all([
                loadDetectors().then(() => {
                    CorridorMapModule.updateDetectorMarkers(detectors);
                    HeatmapModule.updateHeatmap(detectors);
                }),
                GasPanelModule.loadActiveAlarms(),
                loadLeakSources(),
                GasPanelModule.loadStatistics()
            ]);
            
            const selectedDetector = GasPanelModule.getSelectedDetector();
            if (selectedDetector) {
                GasPanelModule.refreshDetectorDetail(selectedDetector.id);
            }
        } catch (error) {
            console.error('刷新数据失败:', error);
        }
    }

    function updateDetectorConcentrations(data) {
        if (!Array.isArray(data)) return;
        
        data.forEach(update => {
            const detector = detectors.find(d => d.id === update.detector_id);
            if (detector) {
                detector.current_concentration = update.concentration;
                detector.last_update = update.timestamp;
            }
        });
        
        CorridorMapModule.updateDetectorMarkers(detectors);
        HeatmapModule.updateHeatmap(detectors);
    }

    function handleLeakSourceUpdate(data) {
        if (data.action === 'new' || data.action === 'update') {
            const existing = leakSources.find(l => l.id === data.leak_source.id);
            if (existing) {
                Object.assign(existing, data.leak_source);
            } else {
                leakSources.push(data.leak_source);
            }
        } else if (data.action === 'resolve') {
            leakSources = leakSources.filter(l => l.id !== data.leak_source_id);
        }
        CorridorMapModule.updateLeakMarkers(leakSources);
    }

    function getDetectors() {
        return detectors;
    }

    function getSelectedDetector() {
        return GasPanelModule.getSelectedDetector();
    }

    return {
        init: init,
        getDetectors: getDetectors,
        getSelectedDetector: getSelectedDetector,
        refreshData: refreshData
    };
})();

document.addEventListener('DOMContentLoaded', function() {
    App.init();
});

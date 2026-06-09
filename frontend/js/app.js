const App = (function() {
    let detectors = [];
    let alarms = [];
    let leakSources = [];
    let selectedDetector = null;
    let refreshInterval = null;

    function init() {
        console.log('初始化应用...');
        
        MapModule.init();
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
            const visible = MapModule.toggleDetectors();
            this.textContent = visible ? '隐藏检测器' : '显示检测器';
        });

        document.getElementById('btn-toggle-heatmap').addEventListener('click', function() {
            const visible = HeatmapModule.toggle();
            this.textContent = visible ? '隐藏热力图' : '显示热力图';
        });

        document.getElementById('btn-toggle-valves').addEventListener('click', function() {
            const visible = MapModule.toggleValves();
            this.textContent = visible ? '隐藏阀门' : '显示阀门';
        });

        document.getElementById('btn-toggle-leaks').addEventListener('click', function() {
            const visible = MapModule.toggleLeaks();
            this.textContent = visible ? '隐藏泄漏源' : '显示泄漏源';
        });

        document.getElementById('btn-acknowledge-alarm').addEventListener('click', acknowledgeSelectedAlarm);
        document.getElementById('btn-close-modal').addEventListener('click', closeDetectorModal);
        
        document.querySelector('.modal-overlay').addEventListener('click', function(e) {
            if (e.target === this) {
                closeDetectorModal();
            }
        });

        document.getElementById('alarm-list').addEventListener('click', function(e) {
            const alarmItem = e.target.closest('.alarm-item');
            if (alarmItem) {
                const alarmId = alarmItem.dataset.alarmId;
                focusOnAlarm(alarmId);
            }
        });
    }

    function setupWebSocketCallbacks() {
        WebSocketModule.setOnConcentrationUpdate(function(data) {
            updateDetectorConcentrations(data);
        });

        WebSocketModule.setOnAlarmUpdate(function(data) {
            handleAlarmUpdate(data);
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
                loadActiveAlarms(),
                loadLeakSources(),
                loadStatistics()
            ]);
            
            MapModule.loadPipeCorridor();
            MapModule.addDetectorMarkers(detectors);
            MapModule.addValveMarkers();
            
            HeatmapModule.updateHeatmap(detectors);
            
        } catch (error) {
            console.error('加载初始数据失败:', error);
            showNotification('加载数据失败，请刷新页面重试', 'error');
        }
    }

    async function loadDetectors() {
        try {
            const response = await fetch(`${Config.API_URL}/detectors`);
            if (!response.ok) throw new Error('获取检测器列表失败');
            detectors = await response.json();
            return detectors;
        } catch (error) {
            console.error('加载检测器失败:', error);
            throw error;
        }
    }

    async function loadActiveAlarms() {
        try {
            const response = await fetch(`${Config.API_URL}/alarms/active`);
            if (!response.ok) throw new Error('获取活动告警失败');
            alarms = await response.json();
            updateAlarmList();
            return alarms;
        } catch (error) {
            console.error('加载活动告警失败:', error);
            throw error;
        }
    }

    async function loadLeakSources() {
        try {
            const response = await fetch(`${Config.API_URL}/leaks/active`);
            if (!response.ok) throw new Error('获取泄漏源失败');
            leakSources = await response.json();
            MapModule.updateLeakMarkers(leakSources);
            return leakSources;
        } catch (error) {
            console.error('加载泄漏源失败:', error);
            throw error;
        }
    }

    async function loadStatistics() {
        try {
            const response = await fetch(`${Config.API_URL}/statistics`);
            if (!response.ok) throw new Error('获取统计数据失败');
            const stats = await response.json();
            updateStatistics(stats);
            return stats;
        } catch (error) {
            console.error('加载统计数据失败:', error);
            throw error;
        }
    }

    async function refreshData() {
        try {
            await Promise.all([
                loadDetectors().then(() => {
                    MapModule.updateDetectorMarkers(detectors);
                    HeatmapModule.updateHeatmap(detectors);
                }),
                loadActiveAlarms(),
                loadLeakSources(),
                loadStatistics()
            ]);
            
            if (selectedDetector) {
                refreshDetectorDetail(selectedDetector.id);
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
        
        MapModule.updateDetectorMarkers(detectors);
        HeatmapModule.updateHeatmap(detectors);
    }

    function handleAlarmUpdate(data) {
        if (data.action === 'new') {
            alarms.push(data.alarm);
            updateAlarmList();
            showAlarmNotification(data.alarm);
        } else if (data.action === 'update') {
            const index = alarms.findIndex(a => a.id === data.alarm.id);
            if (index !== -1) {
                alarms[index] = data.alarm;
                updateAlarmList();
            }
        } else if (data.action === 'resolve') {
            alarms = alarms.filter(a => a.id !== data.alarm_id);
            updateAlarmList();
        }
        
        loadStatistics();
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
        MapModule.updateLeakMarkers(leakSources);
    }

    function updateAlarmList() {
        const listEl = document.getElementById('alarm-list');
        const countEl = document.getElementById('alarm-count');
        
        countEl.textContent = alarms.length;
        
        if (alarms.length === 0) {
            listEl.innerHTML = '<div class="no-alarms">暂无活动告警</div>';
            return;
        }

        const sortedAlarms = [...alarms].sort((a, b) => {
            const levelPriority = { 'level3': 0, 'level2': 1, 'level1': 2 };
            return levelPriority[a.level] - levelPriority[b.level];
        });

        listEl.innerHTML = sortedAlarms.map(alarm => {
            const levelClass = `alarm-${alarm.level}`;
            const levelText = {
                'level1': '一级预警',
                'level2': '二级报警',
                'level3': '三级紧急'
            }[alarm.level] || alarm.level;
            
            const time = new Date(alarm.timestamp * 1000);
            const timeStr = `${time.getHours().toString().padStart(2, '0')}:${time.getMinutes().toString().padStart(2, '0')}:${time.getSeconds().toString().padStart(2, '0')}`;

            return `
                <div class="alarm-item ${levelClass}" data-alarm-id="${alarm.id}">
                    <div class="alarm-header">
                        <span class="alarm-level">${levelText}</span>
                        <span class="alarm-time">${timeStr}</span>
                    </div>
                    <div class="alarm-content">
                        <div class="alarm-device">检测器: ${alarm.detector_id}</div>
                        <div class="alarm-value">浓度: ${alarm.concentration.toFixed(2)}%LEL</div>
                        <div class="alarm-message">${alarm.message || '燃气浓度超标'}</div>
                    </div>
                </div>
            `;
        }).join('');
    }

    function updateStatistics(stats) {
        document.getElementById('stat-total-detectors').textContent = stats.total_detectors || 0;
        document.getElementById('stat-online-detectors').textContent = stats.online_detectors || 0;
        document.getElementById('stat-active-alarms').textContent = stats.active_alarms || 0;
        document.getElementById('stat-leak-sources').textContent = stats.active_leak_sources || 0;
        
        const avgConcentration = stats.avg_concentration || 0;
        document.getElementById('stat-avg-concentration').textContent = avgConcentration.toFixed(2) + '%LEL';
        
        const maxConcentration = stats.max_concentration || 0;
        const maxEl = document.getElementById('stat-max-concentration');
        maxEl.textContent = maxConcentration.toFixed(2) + '%LEL';
        
        if (maxConcentration >= Config.ALARM_LEVEL3) {
            maxEl.className = 'stat-value critical';
        } else if (maxConcentration >= Config.ALARM_LEVEL2) {
            maxEl.className = 'stat-value warning';
        } else if (maxConcentration >= Config.ALARM_LEVEL1) {
            maxEl.className = 'stat-value caution';
        } else {
            maxEl.className = 'stat-value';
        }
    }

    function showAlarmNotification(alarm) {
        const levelText = {
            'level1': '一级预警',
            'level2': '二级报警',
            'level3': '三级紧急'
        }[alarm.level] || alarm.level;

        const message = `${levelText}: 检测器 ${alarm.detector_id} 浓度 ${alarm.concentration.toFixed(2)}%LEL`;
        showNotification(message, alarm.level);

        if (alarm.level === 'level3') {
            playAlarmSound();
        }
    }

    function showNotification(message, type = 'info') {
        const container = document.getElementById('notification-container');
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.innerHTML = `
            <span class="notification-message">${message}</span>
            <button class="notification-close">&times;</button>
        `;
        
        container.appendChild(notification);
        
        notification.querySelector('.notification-close').addEventListener('click', () => {
            notification.remove();
        });
        
        setTimeout(() => {
            if (notification.parentNode) {
                notification.remove();
            }
        }, 8000);
    }

    function playAlarmSound() {
        try {
            const audioContext = new (window.AudioContext || window.webkitAudioContext)();
            const oscillator = audioContext.createOscillator();
            const gainNode = audioContext.createGain();
            
            oscillator.connect(gainNode);
            gainNode.connect(audioContext.destination);
            
            oscillator.frequency.value = 800;
            oscillator.type = 'square';
            gainNode.gain.value = 0.3;
            
            oscillator.start();
            setTimeout(() => oscillator.stop(), 500);
        } catch (e) {
            console.log('无法播放告警声音:', e);
        }
    }

    async function showDetectorDetail(detectorId) {
        const detector = detectors.find(d => d.id === detectorId);
        if (!detector) return;
        
        selectedDetector = detector;
        
        try {
            const [historyData, healthData] = await Promise.all([
                fetch(`${Config.API_URL}/detectors/${detectorId}/history?hours=1`).then(r => r.json()),
                fetch(`${Config.API_URL}/detectors/${detectorId}/health`).then(r => r.json())
            ]);
            
            document.getElementById('modal-detector-id').textContent = detector.id;
            document.getElementById('modal-detector-position').textContent = `里程: ${detector.position_meters}米`;
            document.getElementById('modal-detector-zone').textContent = `防火分区: ${detector.fire_zone_id || '未知'}`;
            document.getElementById('modal-current-concentration').textContent = 
                (detector.current_concentration || 0).toFixed(2) + '%LEL';
            
            const statusText = document.getElementById('modal-detector-status');
            const statusClass = getStatusClass(detector.current_concentration || 0);
            statusText.textContent = statusClass.text;
            statusText.className = `status-badge ${statusClass.class}`;
            
            ChartModule.drawConcentrationChart('concentration-chart', historyData);
            ChartModule.drawHealthIndicator('health-indicator', healthData.status);
            
            document.getElementById('health-status').textContent = healthData.status === 'normal' ? '正常' :
                healthData.status === 'warning' ? '警告' :
                healthData.status === 'error' ? '故障' : '离线';
            document.getElementById('health-last-check').textContent = 
                new Date(healthData.last_check * 1000).toLocaleString('zh-CN');
            document.getElementById('health-uptime').textContent = formatUptime(healthData.uptime_seconds || 0);
            document.getElementById('health-temp').textContent = healthData.temperature ? 
                healthData.temperature.toFixed(1) + '°C' : 'N/A';
            document.getElementById('health-voltage').textContent = healthData.voltage ? 
                healthData.voltage.toFixed(2) + 'V' : 'N/A';
            document.getElementById('health-signal').textContent = healthData.signal_strength ? 
                healthData.signal_strength + '%' : 'N/A';
            
            document.getElementById('detector-modal').style.display = 'block';
            
        } catch (error) {
            console.error('加载检测器详情失败:', error);
            showNotification('加载检测器详情失败', 'error');
        }
    }

    async function refreshDetectorDetail(detectorId) {
        if (!selectedDetector || selectedDetector.id !== detectorId) return;
        
        try {
            const [historyData, healthData] = await Promise.all([
                fetch(`${Config.API_URL}/detectors/${detectorId}/history?hours=1`).then(r => r.json()),
                fetch(`${Config.API_URL}/detectors/${detectorId}/health`).then(r => r.json())
            ]);
            
            const detector = detectors.find(d => d.id === detectorId);
            if (detector) {
                document.getElementById('modal-current-concentration').textContent = 
                    (detector.current_concentration || 0).toFixed(2) + '%LEL';
                
                const statusClass = getStatusClass(detector.current_concentration || 0);
                const statusText = document.getElementById('modal-detector-status');
                statusText.textContent = statusClass.text;
                statusText.className = `status-badge ${statusClass.class}`;
            }
            
            ChartModule.drawConcentrationChart('concentration-chart', historyData);
            ChartModule.drawHealthIndicator('health-indicator', healthData.status);
            
        } catch (error) {
            console.error('刷新检测器详情失败:', error);
        }
    }

    function closeDetectorModal() {
        document.getElementById('detector-modal').style.display = 'none';
        selectedDetector = null;
    }

    function getStatusClass(concentration) {
        if (concentration >= Config.ALARM_LEVEL3) {
            return { text: '三级紧急', class: 'status-danger' };
        } else if (concentration >= Config.ALARM_LEVEL2) {
            return { text: '二级报警', class: 'status-warning' };
        } else if (concentration >= Config.ALARM_LEVEL1) {
            return { text: '一级预警', class: 'status-caution' };
        } else {
            return { text: '正常', class: 'status-normal' };
        }
    }

    function formatUptime(seconds) {
        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        
        if (days > 0) {
            return `${days}天${hours}小时${minutes}分钟`;
        } else if (hours > 0) {
            return `${hours}小时${minutes}分钟`;
        } else {
            return `${minutes}分钟`;
        }
    }

    function focusOnAlarm(alarmId) {
        const alarm = alarms.find(a => a.id === alarmId);
        if (alarm && alarm.detector_id) {
            const detector = detectors.find(d => d.id === alarm.detector_id);
            if (detector && detector.latitude && detector.longitude) {
                window.map.setView([detector.latitude, detector.longitude], 15);
                showDetectorDetail(alarm.detector_id);
            }
        }
    }

    async function acknowledgeSelectedAlarm() {
        const checkboxes = document.querySelectorAll('.alarm-item input[type="checkbox"]:checked');
        if (checkboxes.length === 0) {
            showNotification('请先选择要确认的告警', 'info');
            return;
        }

        for (const checkbox of checkboxes) {
            const alarmId = checkbox.closest('.alarm-item').dataset.alarmId;
            try {
                await fetch(`${Config.API_URL}/alarms/${alarmId}/acknowledge`, {
                    method: 'POST'
                });
            } catch (error) {
                console.error('确认告警失败:', error);
            }
        }
        
        loadActiveAlarms();
        showNotification(`已确认 ${checkboxes.length} 条告警`, 'success');
    }

    function getDetectors() {
        return detectors;
    }

    function getSelectedDetector() {
        return selectedDetector;
    }

    return {
        init: init,
        showDetectorDetail: showDetectorDetail,
        getDetectors: getDetectors,
        getSelectedDetector: getSelectedDetector,
        refreshData: refreshData
    };
})();

document.addEventListener('DOMContentLoaded', function() {
    App.init();
});

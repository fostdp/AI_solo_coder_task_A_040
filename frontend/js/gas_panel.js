const GasPanelModule = (function() {
    let detectors = [];
    let alarms = [];
    let selectedDetector = null;

    function init() {
        console.log('初始化浓度面板模块...');
    }

    function setDetectors(detectorList) {
        detectors = detectorList;
    }

    function setAlarms(alarmList) {
        alarms = alarmList;
        updateAlarmList();
    }

    function getSelectedDetector() {
        return selectedDetector;
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

    function updateAlarmList() {
        const listEl = document.getElementById('alarm-list');
        const countEl = document.getElementById('alarm-count');
        
        if (!listEl || !countEl) return;
        
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
        const totalDetectorsEl = document.getElementById('stat-total-detectors');
        const onlineDetectorsEl = document.getElementById('stat-online-detectors');
        const activeAlarmsEl = document.getElementById('stat-active-alarms');
        const leakSourcesEl = document.getElementById('stat-leak-sources');
        const avgConcentrationEl = document.getElementById('stat-avg-concentration');
        const maxConcentrationEl = document.getElementById('stat-max-concentration');
        
        if (totalDetectorsEl) totalDetectorsEl.textContent = stats.total_detectors || 0;
        if (onlineDetectorsEl) onlineDetectorsEl.textContent = stats.online_detectors || 0;
        if (activeAlarmsEl) activeAlarmsEl.textContent = stats.active_alarms || 0;
        if (leakSourcesEl) leakSourcesEl.textContent = stats.active_leak_sources || 0;
        
        const avgConcentration = stats.avg_concentration || 0;
        if (avgConcentrationEl) avgConcentrationEl.textContent = avgConcentration.toFixed(2) + '%LEL';
        
        const maxConcentration = stats.max_concentration || 0;
        if (maxConcentrationEl) {
            maxConcentrationEl.textContent = maxConcentration.toFixed(2) + '%LEL';
            
            if (maxConcentration >= Config.ALARM_LEVEL3) {
                maxConcentrationEl.className = 'stat-value critical';
            } else if (maxConcentration >= Config.ALARM_LEVEL2) {
                maxConcentrationEl.className = 'stat-value warning';
            } else if (maxConcentration >= Config.ALARM_LEVEL1) {
                maxConcentrationEl.className = 'stat-value caution';
            } else {
                maxConcentrationEl.className = 'stat-value';
            }
        }
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
        if (!container) return;
        
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
            
            const modalDetectorIdEl = document.getElementById('modal-detector-id');
            const modalDetectorPositionEl = document.getElementById('modal-detector-position');
            const modalDetectorZoneEl = document.getElementById('modal-detector-zone');
            const modalCurrentConcentrationEl = document.getElementById('modal-current-concentration');
            const modalDetectorStatusEl = document.getElementById('modal-detector-status');
            
            if (modalDetectorIdEl) modalDetectorIdEl.textContent = detector.id;
            if (modalDetectorPositionEl) modalDetectorPositionEl.textContent = `里程: ${detector.position_meters}米`;
            if (modalDetectorZoneEl) modalDetectorZoneEl.textContent = `防火分区: ${detector.fire_zone_id || '未知'}`;
            if (modalCurrentConcentrationEl) {
                modalCurrentConcentrationEl.textContent = 
                    (detector.current_concentration || 0).toFixed(2) + '%LEL';
            }
            
            if (modalDetectorStatusEl) {
                const statusClass = getStatusClass(detector.current_concentration || 0);
                modalDetectorStatusEl.textContent = statusClass.text;
                modalDetectorStatusEl.className = `status-badge ${statusClass.class}`;
            }
            
            if (typeof ChartModule !== 'undefined') {
                ChartModule.drawConcentrationChart('concentration-chart', historyData);
                ChartModule.drawHealthIndicator('health-indicator', healthData.status);
            }
            
            const healthStatusEl = document.getElementById('health-status');
            const healthLastCheckEl = document.getElementById('health-last-check');
            const healthUptimeEl = document.getElementById('health-uptime');
            const healthTempEl = document.getElementById('health-temp');
            const healthVoltageEl = document.getElementById('health-voltage');
            const healthSignalEl = document.getElementById('health-signal');
            
            if (healthStatusEl) {
                healthStatusEl.textContent = healthData.status === 'normal' ? '正常' :
                    healthData.status === 'warning' ? '警告' :
                    healthData.status === 'error' ? '故障' : '离线';
            }
            if (healthLastCheckEl) {
                healthLastCheckEl.textContent = 
                    new Date(healthData.last_check * 1000).toLocaleString('zh-CN');
            }
            if (healthUptimeEl) {
                healthUptimeEl.textContent = formatUptime(healthData.uptime_seconds || 0);
            }
            if (healthTempEl) {
                healthTempEl.textContent = healthData.temperature ? 
                    healthData.temperature.toFixed(1) + '°C' : 'N/A';
            }
            if (healthVoltageEl) {
                healthVoltageEl.textContent = healthData.voltage ? 
                    healthData.voltage.toFixed(2) + 'V' : 'N/A';
            }
            if (healthSignalEl) {
                healthSignalEl.textContent = healthData.signal_strength ? 
                    healthData.signal_strength + '%' : 'N/A';
            }
            
            const modalEl = document.getElementById('detector-modal');
            if (modalEl) modalEl.style.display = 'block';
            
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
                const modalCurrentConcentrationEl = document.getElementById('modal-current-concentration');
                const modalDetectorStatusEl = document.getElementById('modal-detector-status');
                
                if (modalCurrentConcentrationEl) {
                    modalCurrentConcentrationEl.textContent = 
                        (detector.current_concentration || 0).toFixed(2) + '%LEL';
                }
                
                if (modalDetectorStatusEl) {
                    const statusClass = getStatusClass(detector.current_concentration || 0);
                    modalDetectorStatusEl.textContent = statusClass.text;
                    modalDetectorStatusEl.className = `status-badge ${statusClass.class}`;
                }
            }
            
            if (typeof ChartModule !== 'undefined') {
                ChartModule.drawConcentrationChart('concentration-chart', historyData);
                ChartModule.drawHealthIndicator('health-indicator', healthData.status);
            }
            
        } catch (error) {
            console.error('刷新检测器详情失败:', error);
        }
    }

    function closeDetectorModal() {
        const modalEl = document.getElementById('detector-modal');
        if (modalEl) modalEl.style.display = 'none';
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
                if (window.map) {
                    window.map.setView([detector.latitude, detector.longitude], 15);
                }
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

    return {
        init: init,
        setDetectors: setDetectors,
        setAlarms: setAlarms,
        getSelectedDetector: getSelectedDetector,
        loadActiveAlarms: loadActiveAlarms,
        loadStatistics: loadStatistics,
        updateAlarmList: updateAlarmList,
        updateStatistics: updateStatistics,
        handleAlarmUpdate: handleAlarmUpdate,
        showAlarmNotification: showAlarmNotification,
        showNotification: showNotification,
        showDetectorDetail: showDetectorDetail,
        refreshDetectorDetail: refreshDetectorDetail,
        closeDetectorModal: closeDetectorModal,
        focusOnAlarm: focusOnAlarm,
        acknowledgeSelectedAlarm: acknowledgeSelectedAlarm,
        getStatusClass: getStatusClass,
        formatUptime: formatUptime
    };
})();

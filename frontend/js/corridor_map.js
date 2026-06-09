const CorridorMapModule = (function() {
    let detectorMarkers = {};
    let valveMarkers = {};
    let leakMarkers = {};
    let leakCircles = {};
    let pipeCorridorLayer = null;
    let detectorsVisible = true;
    let valvesVisible = true;
    let leaksVisible = true;
    
    let clusterMarkers = {};
    let isClusteringEnabled = true;
    let clusterThreshold = 12;
    let clusterGridSize = 80;
    
    let currentViewport = null;
    let viewportUpdateScheduled = false;
    let allDetectorsCache = [];

    function init() {
        window.map = L.map('map', {
            center: Config.MAP_CENTER,
            zoom: Config.MAP_ZOOM,
            preferCanvas: true,
            zoomControl: true,
            attributionControl: true,
            renderer: L.canvas()
        });

        L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
            attribution: '© OpenStreetMap contributors',
            maxZoom: 19,
            className: 'map-tiles'
        }).addTo(window.map);

        window.map.on('moveend', handleViewportChange);
        window.map.on('zoomend', handleViewportChange);
    }

    function handleViewportChange() {
        if (viewportUpdateScheduled) return;
        
        viewportUpdateScheduled = true;
        setTimeout(() => {
            currentViewport = window.map.getBounds();
            updateVisibleDetectors();
            viewportUpdateScheduled = false;
        }, 100);
    }

    function isInViewport(lat, lng) {
        if (!currentViewport) return true;
        return currentViewport.contains([lat, lng]);
    }

    function updateVisibleDetectors() {
        if (!detectorsVisible || allDetectorsCache.length === 0) return;

        const zoom = window.map.getZoom();
        const shouldCluster = isClusteringEnabled && zoom < clusterThreshold;

        if (shouldCluster) {
            updateClusterMarkers();
        } else {
            clearClusterMarkers();
            updateIndividualMarkers();
        }
    }

    function updateIndividualMarkers() {
        let visibleCount = 0;
        let hiddenCount = 0;

        allDetectorsCache.forEach(detector => {
            if (!detector.latitude || !detector.longitude) return;

            const inView = isInViewport(detector.latitude, detector.longitude);
            const markerData = detectorMarkers[detector.id];

            if (inView) {
                visibleCount++;
                if (!markerData) {
                    addDetectorMarker(detector);
                }
            } else {
                hiddenCount++;
                if (markerData && markerData.marker) {
                    window.map.removeLayer(markerData.marker);
                    delete detectorMarkers[detector.id];
                }
            }
        });

        if (visibleCount + hiddenCount > 0) {
            console.log(`视口内检测器: ${visibleCount}, 视口外已隐藏: ${hiddenCount}`);
        }
    }

    function updateClusterMarkers() {
        clearClusterMarkers();
        clearIndividualMarkers();

        const clusters = clusterDetectors(allDetectorsCache);
        let clusterCount = 0;

        clusters.forEach(cluster => {
            if (cluster.count === 1) {
                addDetectorMarker(cluster.detectors[0]);
            } else {
                addClusterMarker(cluster);
                clusterCount++;
            }
        });

        if (clusterCount > 0) {
            console.log(`聚合显示: ${clusterCount}个聚合点, 共${allDetectorsCache.length}个检测器`);
        }
    }

    function clusterDetectors(detectors) {
        const clusters = [];
        const mapZoom = window.map.getZoom();
        const gridSize = clusterGridSize / Math.pow(2, mapZoom - 10);

        const grid = {};

        detectors.forEach(detector => {
            if (!detector.latitude || !detector.longitude) return;
            
            const point = window.map.latLngToContainerPoint([detector.latitude, detector.longitude]);
            const gridX = Math.floor(point.x / gridSize);
            const gridY = Math.floor(point.y / gridSize);
            const gridKey = `${gridX}_${gridY}`;

            if (!grid[gridKey]) {
                grid[gridKey] = {
                    detectors: [],
                    sumLat: 0,
                    sumLng: 0,
                    maxConc: 0,
                    alarmCount: 0
                };
            }

            grid[gridKey].detectors.push(detector);
            grid[gridKey].sumLat += detector.latitude;
            grid[gridKey].sumLng += detector.longitude;
            grid[gridKey].maxConc = Math.max(grid[gridKey].maxConc, detector.current_concentration || 0);
            if (detector.current_concentration >= Config.ALARM_LEVEL1) {
                grid[gridKey].alarmCount++;
            }
        });

        Object.values(grid).forEach(gridData => {
            if (gridData.detectors.length === 0) return;

            const centerLat = gridData.sumLat / gridData.detectors.length;
            const centerLng = gridData.sumLng / gridData.detectors.length;
            const maxConc = gridData.maxConc;
            const count = gridData.detectors.length;
            const alarmCount = gridData.alarmCount;

            clusters.push({
                count: count,
                alarmCount: alarmCount,
                maxConc: maxConc,
                centerLat: centerLat,
                centerLng: centerLng,
                detectors: gridData.detectors
            });
        });

        return clusters;
    }

    function addClusterMarker(cluster) {
        const color = getConcentrationColor(cluster.maxConc);
        const isAlarm = cluster.alarmCount > 0;
        const size = Math.min(40, 16 + cluster.count * 0.5);

        const icon = L.divIcon({
            className: 'custom-marker cluster-marker',
            html: `
                <div class="cluster-marker-inner ${isAlarm ? 'alarm' : ''}" 
                     style="background: ${color}; width: ${size}px; height: ${size}px; line-height: ${size}px;">
                    ${cluster.count}
                    ${cluster.alarmCount > 0 ? `<span class="cluster-alarm-badge">${cluster.alarmCount}</span>` : ''}
                </div>
            `,
            iconSize: [size, size],
            iconAnchor: [size / 2, size / 2]
        });

        const marker = L.marker([cluster.centerLat, cluster.centerLng], {
            icon: icon,
            title: `聚合点 (${cluster.count}个检测器)`
        });

        const detectorsList = cluster.detectors.map(d => 
            `• ${d.id}: ${(d.current_concentration || 0).toFixed(2)}%LEL`
        ).join('<br>');

        marker.bindTooltip(`
            <div class="tooltip-content">
                <strong>聚合点 (${cluster.count}个检测器)</strong><br>
                <span style="color: ${color}; font-weight: bold;">
                    最高浓度: ${cluster.maxConc.toFixed(2)}% LEL
                </span>
                <br>
                ${cluster.alarmCount > 0 ? `<span style="color: #ef4444;">告警: ${cluster.alarmCount}个</span><br>` : ''}
                <div style="margin-top: 4px; font-size: 11px; max-height: 100px; overflow-y: auto;">
                    ${detectorsList}
                </div>
            </div>
        `, {
            permanent: false,
            direction: 'top',
            offset: [0, -10]
        });

        marker.on('click', () => {
            window.map.flyTo([cluster.centerLat, cluster.centerLng], window.map.getZoom() + 2);
        });

        marker.addTo(window.map);
        
        const key = `cluster_${cluster.centerLat.toFixed(6)}_${cluster.centerLng.toFixed(6)}`;
        clusterMarkers[key] = { marker, cluster };
    }

    function clearClusterMarkers() {
        Object.values(clusterMarkers).forEach(({ marker }) => {
            if (marker && window.map.hasLayer(marker)) {
                window.map.removeLayer(marker);
            }
        });
        clusterMarkers = {};
    }

    function clearIndividualMarkers() {
        Object.values(detectorMarkers).forEach(({ marker }) => {
            if (marker && window.map.hasLayer(marker)) {
                window.map.removeLayer(marker);
            }
        });
        detectorMarkers = {};
    }

    async function loadPipeCorridor() {
        try {
            const response = await fetch(`${Config.API_URL}/pipe-corridor`);
            if (!response.ok) throw new Error('获取管廊路径失败');
            const data = await response.json();
            
            if (data.length > 0) {
                const latlngs = data.map(p => [p.latitude, p.longitude]);

                if (pipeCorridorLayer) {
                    window.map.removeLayer(pipeCorridorLayer);
                }

                const boundary = L.polyline(latlngs, {
                    color: '#1e40af',
                    weight: 10,
                    opacity: 0.3,
                    lineJoin: 'round',
                    lineCap: 'round',
                    interactive: false,
                    pane: 'tilePane'
                }).addTo(window.map);

                pipeCorridorLayer = L.polyline(latlngs, {
                    color: '#3b82f6',
                    weight: 6,
                    opacity: 0.8,
                    lineJoin: 'round',
                    lineCap: 'round',
                    interactive: false,
                    pane: 'tilePane'
                }).addTo(window.map);

                const bounds = L.latLngBounds(latlngs);
                window.map.fitBounds(bounds, { padding: [50, 50] });
            }
        } catch (error) {
            console.error('加载管廊路径失败:', error);
        }
    }

    function addDetectorMarkers(detectors) {
        allDetectorsCache = detectors;
        clearDetectorMarkers();
        clearClusterMarkers();
        
        currentViewport = window.map.getBounds();
        updateVisibleDetectors();
    }

    function addDetectorMarker(detector) {
        const concentration = detector.current_concentration || 0;
        const color = getConcentrationColor(concentration);
        const isAlarm = concentration >= Config.ALARM_LEVEL1;

        const icon = L.divIcon({
            className: 'custom-marker',
            html: `<div class="detector-marker ${isAlarm ? 'alarm' : ''}" style="background: ${color};"></div>`,
            iconSize: [16, 16],
            iconAnchor: [8, 8]
        });

        const marker = L.marker([detector.latitude, detector.longitude], {
            icon: icon,
            title: detector.id
        });

        marker.bindTooltip(`
            <div class="tooltip-content">
                <strong>检测器 ${detector.id}</strong><br>
                <span style="color: ${color}; font-weight: bold;">${concentration.toFixed(2)}% LEL</span><br>
                <span style="font-size: 11px; color: #6b7280;">
                    位置: ${detector.position_meters || 0}m<br>
                    分区: ${detector.fire_zone_id || '未知'}
                </span>
            </div>
        `, {
            permanent: false,
            direction: 'top',
            offset: [0, -10]
        });

        marker.on('click', () => {
            if (window.GasPanelModule && typeof window.GasPanelModule.showDetectorDetail === 'function') {
                window.GasPanelModule.showDetectorDetail(detector.id);
            } else if (window.App && typeof window.App.showDetectorDetail === 'function') {
                window.App.showDetectorDetail(detector.id);
            }
        });

        marker.addTo(window.map);
        
        detectorMarkers[detector.id] = {
            marker: marker,
            detector: detector
        };
    }

    function updateDetectorMarkers(detectors) {
        allDetectorsCache = detectors;
        
        const zoom = window.map.getZoom();
        const shouldCluster = isClusteringEnabled && zoom < clusterThreshold;

        if (shouldCluster) {
            updateClusterMarkers();
            return;
        }

        detectors.forEach(detector => {
            if (!detector.latitude || !detector.longitude) return;

            const inView = isInViewport(detector.latitude, detector.longitude);
            if (!inView) return;

            const concentration = detector.current_concentration || 0;
            const color = getConcentrationColor(concentration);
            const isAlarm = concentration >= Config.ALARM_LEVEL1;

            const markerData = detectorMarkers[detector.id];
            if (markerData) {
                const icon = L.divIcon({
                    className: 'custom-marker',
                    html: `<div class="detector-marker ${isAlarm ? 'alarm' : ''}" style="background: ${color};"></div>`,
                    iconSize: [16, 16],
                    iconAnchor: [8, 8]
                });

                markerData.marker.setIcon(icon);
                markerData.detector.current_concentration = concentration;

                markerData.marker.setTooltipContent(`
                    <div class="tooltip-content">
                        <strong>检测器 ${detector.id}</strong><br>
                        <span style="color: ${color}; font-weight: bold;">${concentration.toFixed(2)}% LEL</span><br>
                        <span style="font-size: 11px; color: #6b7280;">
                            位置: ${detector.position_meters || 0}m<br>
                            分区: ${detector.fire_zone_id || '未知'}
                        </span>
                    </div>
                `);
            } else {
                addDetectorMarker(detector);
            }
        });
    }

    function clearDetectorMarkers() {
        clearIndividualMarkers();
    }

    async function addValveMarkers() {
        try {
            const response = await fetch(`${Config.API_URL}/valves`);
            if (!response.ok) throw new Error('获取阀门列表失败');
            const valves = await response.json();
            
            clearValveMarkers();
            
            valves.forEach(valve => {
                if (valve.latitude && valve.longitude) {
                    addValveMarker(valve);
                }
            });
        } catch (error) {
            console.error('加载阀门列表失败:', error);
        }
    }

    function addValveMarker(valve) {
        const isClosed = valve.status === 'closed';
        const color = isClosed ? Config.COLORS.VALVE_CLOSED : Config.COLORS.VALVE;

        const icon = L.divIcon({
            className: 'custom-marker',
            html: `<div class="valve-marker ${isClosed ? 'closed' : ''}" style="background: ${color};"></div>`,
            iconSize: [14, 14],
            iconAnchor: [7, 7]
        });

        const marker = L.marker([valve.latitude, valve.longitude], {
            icon: icon,
            title: valve.name || valve.id
        });

        marker.bindTooltip(`
            <div class="tooltip-content">
                <strong>${valve.name || '阀门 ' + valve.id}</strong><br>
                <span style="color: ${color}; font-weight: bold;">
                    ${isClosed ? '已关闭' : '已开启'}
                </span><br>
                <span style="font-size: 11px; color: #6b7280;">
                    分区: ${valve.fire_zone_id || '未知'}
                </span>
            </div>
        `, {
            permanent: false,
            direction: 'top',
            offset: [0, -10]
        });

        if (valvesVisible) {
            marker.addTo(window.map);
        }
        
        valveMarkers[valve.id] = {
            marker: marker,
            valve: valve
        };
    }

    function clearValveMarkers() {
        Object.values(valveMarkers).forEach(({ marker }) => {
            window.map.removeLayer(marker);
        });
        valveMarkers = {};
    }

    function updateLeakMarkers(leaks) {
        const existingIds = new Set(Object.keys(leakMarkers));
        const newIds = new Set(leaks.map(l => l.id.toString()));

        leaks.forEach(leak => {
            const leakId = leak.id.toString();
            if (!existingIds.has(leakId)) {
                addLeakMarker(leak);
            } else {
                if (leakCircles[leakId] && leak.diffusion_radius) {
                    leakCircles[leakId].setRadius(leak.diffusion_radius);
                }
            }
        });

        existingIds.forEach(id => {
            if (!newIds.has(id)) {
                removeLeakMarker(id);
            }
        });
    }

    function addLeakMarker(leak) {
        const leakId = leak.id.toString();
        
        if (leakMarkers[leakId]) {
            removeLeakMarker(leakId);
        }

        const icon = L.divIcon({
            className: 'custom-marker',
            html: '<div class="leak-marker"></div>',
            iconSize: [24, 24],
            iconAnchor: [12, 12]
        });

        const marker = L.marker([leak.latitude, leak.longitude], {
            icon: icon,
            title: '泄漏源'
        });

        marker.bindTooltip(`
            <div class="tooltip-content">
                <strong style="color: #ef4444;">⚠️ 泄漏源</strong><br>
                <span>位置: ${(leak.position_meters || 0).toFixed(0)}m</span><br>
                <span>速率: ${(leak.leak_rate || 0).toFixed(4)} L/s</span><br>
                <span>置信度: ${(leak.confidence || 0).toFixed(1)}%</span>
            </div>
        `, {
            permanent: false,
            direction: 'top',
            offset: [0, -15]
        });

        if (leaksVisible) {
            marker.addTo(window.map);
        }

        const circle = L.circle([leak.latitude, leak.longitude], {
            radius: leak.diffusion_radius || 50,
            color: '#ef4444',
            fillColor: '#ef4444',
            fillOpacity: 0.15,
            weight: 2,
            dashArray: '10, 5',
            opacity: 0.6,
            interactive: false
        });

        if (leaksVisible) {
            circle.addTo(window.map);
        }

        leakMarkers[leakId] = { marker, leak };
        leakCircles[leakId] = circle;
    }

    function removeLeakMarker(leakId) {
        if (leakMarkers[leakId]) {
            window.map.removeLayer(leakMarkers[leakId].marker);
            delete leakMarkers[leakId];
        }
        if (leakCircles[leakId]) {
            window.map.removeLayer(leakCircles[leakId]);
            delete leakCircles[leakId];
        }
    }

    function getConcentrationColor(concentration) {
        if (concentration >= Config.ALARM_LEVEL3) {
            return Config.COLORS.LEVEL3;
        } else if (concentration >= Config.ALARM_LEVEL2) {
            return Config.COLORS.LEVEL2;
        } else if (concentration >= Config.ALARM_LEVEL1) {
            return Config.COLORS.LEVEL1;
        }
        return Config.COLORS.NORMAL;
    }

    function toggleDetectors() {
        detectorsVisible = !detectorsVisible;
        
        if (detectorsVisible) {
            currentViewport = window.map.getBounds();
            updateVisibleDetectors();
        } else {
            clearIndividualMarkers();
            clearClusterMarkers();
        }
        return detectorsVisible;
    }

    function toggleValves() {
        valvesVisible = !valvesVisible;
        Object.values(valveMarkers).forEach(({ marker }) => {
            if (valvesVisible) {
                marker.addTo(window.map);
            } else {
                window.map.removeLayer(marker);
            }
        });
        return valvesVisible;
    }

    function toggleLeaks() {
        leaksVisible = !leaksVisible;
        Object.values(leakMarkers).forEach(({ marker }) => {
            if (leaksVisible) {
                marker.addTo(window.map);
            } else {
                window.map.removeLayer(marker);
            }
        });
        Object.values(leakCircles).forEach(circle => {
            if (leaksVisible) {
                circle.addTo(window.map);
            } else {
                window.map.removeLayer(circle);
            }
        });
        return leaksVisible;
    }

    function showDetectors(show) {
        detectorsVisible = show;
        if (show) {
            currentViewport = window.map.getBounds();
            updateVisibleDetectors();
        } else {
            clearIndividualMarkers();
            clearClusterMarkers();
        }
    }

    function showValves(show) {
        valvesVisible = show;
        Object.values(valveMarkers).forEach(({ marker }) => {
            if (show) {
                marker.addTo(window.map);
            } else {
                window.map.removeLayer(marker);
            }
        });
    }

    function showLeaks(show) {
        leaksVisible = show;
        Object.values(leakMarkers).forEach(({ marker }) => {
            if (show) {
                marker.addTo(window.map);
            } else {
                window.map.removeLayer(marker);
            }
        });
        Object.values(leakCircles).forEach(circle => {
            if (show) {
                circle.addTo(window.map);
            } else {
                window.map.removeLayer(circle);
            }
        });
    }

    function getDetectorById(id) {
        return detectorMarkers[id]?.detector || 
               allDetectorsCache.find(d => d.id === id);
    }

    function getAllDetectors() {
        return [...allDetectorsCache];
    }

    return {
        init: init,
        loadPipeCorridor: loadPipeCorridor,
        addDetectorMarkers: addDetectorMarkers,
        addDetectorMarker: addDetectorMarker,
        updateDetectorMarkers: updateDetectorMarkers,
        addValveMarkers: addValveMarkers,
        addValveMarker: addValveMarker,
        updateLeakMarkers: updateLeakMarkers,
        addLeakMarker: addLeakMarker,
        removeLeakMarker: removeLeakMarker,
        getConcentrationColor: getConcentrationColor,
        toggleDetectors: toggleDetectors,
        toggleValves: toggleValves,
        toggleLeaks: toggleLeaks,
        showDetectors: showDetectors,
        showValves: showValves,
        showLeaks: showLeaks,
        getDetectorById: getDetectorById,
        getAllDetectors: getAllDetectors
    };
})();

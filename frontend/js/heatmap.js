const HeatmapModule = (function() {
    let heatmapLayer = null;
    let isVisible = true;

    function init() {
        heatmapLayer = L.layerGroup().addTo(window.map);
    }

    function updateHeatmap(detectors) {
        if (!heatmapLayer) return;
        
        heatmapLayer.clearLayers();

        detectors.forEach(detector => {
            if (detector.latitude && detector.longitude && isVisible) {
                const concentration = detector.current_concentration || 0;
                const intensity = Math.min(concentration / Config.HEATMAP_MAX_CONCENTRATION, 1);
                
                if (intensity > 0.05) {
                    const color = getHeatColor(intensity);
                    const radius = 8 + intensity * 25;
                    const opacity = 0.2 + intensity * 0.5;

                    const heatCircle = L.circleMarker(
                        [detector.latitude, detector.longitude],
                        {
                            radius: radius,
                            fillColor: color,
                            color: color,
                            weight: 0,
                            fillOpacity: opacity,
                            interactive: false
                        }
                    );
                    heatmapLayer.addLayer(heatCircle);
                }
            }
        });
    }

    function getHeatColor(value) {
        if (value < 0.2) {
            return `rgba(0, 255, 0, ${0.3 + value})`;
        } else if (value < 0.4) {
            return `rgba(255, 255, 0, ${0.4 + value * 0.5})`;
        } else if (value < 0.6) {
            return `rgba(255, 165, 0, ${0.5 + value * 0.5})`;
        } else if (value < 0.8) {
            return `rgba(255, 100, 0, ${0.6 + value * 0.4})`;
        } else {
            return `rgba(255, 0, 0, ${0.7 + value * 0.3})`;
        }
    }

    function show() {
        isVisible = true;
        if (heatmapLayer) {
            heatmapLayer.addTo(window.map);
        }
    }

    function hide() {
        isVisible = false;
        if (heatmapLayer) {
            heatmapLayer.remove();
        }
    }

    function toggle() {
        if (isVisible) {
            hide();
        } else {
            show();
        }
        return isVisible;
    }

    function clear() {
        if (heatmapLayer) {
            heatmapLayer.clearLayers();
        }
    }

    return {
        init: init,
        updateHeatmap: updateHeatmap,
        show: show,
        hide: hide,
        toggle: toggle,
        clear: clear
    };
})();

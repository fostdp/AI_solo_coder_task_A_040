const Config = {
    API_URL: '/api',
    WS_URL: `ws://${window.location.host}/api/ws`,
    MAP_CENTER: [39.9142, 116.4274],
    MAP_ZOOM: 13,
    
    ALARM_LEVEL1: 10,
    ALARM_LEVEL2: 20,
    ALARM_LEVEL3: 50,
    
    HEATMAP_MAX_CONCENTRATION: 60,
    MAX_CONCENTRATION: 100,
    
    COLORS: {
        NORMAL: '#10b981',
        LEVEL1: '#fbbf24',
        LEVEL2: '#f97316',
        LEVEL3: '#ef4444',
        LEAK: '#ef4444',
        VALVE: '#8b5cf6',
        VALVE_CLOSED: '#ef4444'
    }
};
